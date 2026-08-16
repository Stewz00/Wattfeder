// Package application runs the single-household edge runtime data flow.
package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Stewz00/wattfeder/internal/household"
	"github.com/Stewz00/wattfeder/internal/persistence"
)

// Record is the additive flat outcome of processing one interval, including intervals that
// produced no telemetry or command. Timestamp, the measurement fields, and CommandPowerKW are
// nil precisely when the corresponding data does not exist for this interval's disposition;
// Decision, DispositionReason, HealthReason, and Reason are empty strings under the same
// condition. AgentID is empty when the agent was configured without an identity.
type Record struct {
	AgentID            string                       `json:"agent_id,omitempty"`
	DeviceID           string                       `json:"device_id"`
	ReceivedAt         time.Time                    `json:"received_at"`
	Disposition        household.Disposition        `json:"disposition"`
	DispositionReason  string                       `json:"disposition_reason,omitempty"`
	EventID            household.EventID            `json:"event_id,omitempty"`
	Timestamp          *time.Time                   `json:"timestamp,omitempty"`
	PVPowerKW          *float64                     `json:"pv_power_kw,omitempty"`
	LoadPowerKW        *float64                     `json:"load_power_kw,omitempty"`
	BatterySOCPercent  *float64                     `json:"battery_soc_percent,omitempty"`
	PriceEURPerKWh     *float64                     `json:"electricity_price_eur_kwh,omitempty"`
	StoredHistory      bool                         `json:"stored_history"`
	StateUpdated       bool                         `json:"state_updated"`
	HealthStatus       household.DeviceHealthStatus `json:"health_status"`
	HealthReason       string                       `json:"health_reason,omitempty"`
	HealthTransitionAt time.Time                    `json:"health_transition_at"`
	Decision           household.Decision           `json:"decision,omitempty"`
	CommandPowerKW     *float64                     `json:"command_power_kw,omitempty"`
	Reason             string                       `json:"reason,omitempty"`
}

// IsZero reports whether this Record describes nothing. Every interval that runs to completion
// produces a Record with at least a device ID and a disposition, so the zero value only ever
// reaches an observer when the interval ended before producing a result.
func (r Record) IsZero() bool {
	return r == Record{}
}

// Agent is everything one edge agent needs to process telemetry for one household. Run rejects
// a missing Repository or a blank DeviceID; the fields whose meaning is not carried by their
// type document their own contract.
type Agent struct {
	Clock      Clock
	Source     TelemetrySource
	Sink       CommandSink
	Policy     household.Policy
	Repository persistence.Repository

	// Observer attaches an adapter to interval processing — logging, metrics, tracing. A nil
	// Observer disables observation entirely.
	Observer Observer

	// ID identifies this installed agent instance in every emitted Record. It is a runtime
	// value only: nothing persists it or reads it back out of storage.
	ID string

	// DeviceID is the household system this agent manages, and the only identity that reaches
	// storage.
	DeviceID string

	// MaxIntervals bounds how many intervals the run processes. Zero or less runs until
	// cancellation or an exhausted source ends it.
	MaxIntervals int

	// ShutdownGrace bounds how long an interval that was already classified may take to commit
	// and apply its command once cancellation has arrived.
	ShutdownGrace time.Duration

	Write func(Record) error
}

// Run processes telemetry interval by interval, classifying and durably committing every
// observation before deciding whether to apply its command. It continues past duplicate,
// historical, delayed, incomplete, invalid, missing-heartbeat, and unavailable observations,
// writing one Record per interval regardless of disposition. A run ends cleanly when the
// context is cancelled, when the source returns ErrSourceExhausted, or when Agent.MaxIntervals
// is positive and that many intervals have been processed. Any other persistence, sink, or
// write error stops the run and reports failure.
//
// Cancellation is checked once per interval, before the source is asked for the next
// observation, so an event is abandoned only if it was never classified. Once classified, the
// observation is committed and its command applied on a context derived with
// context.WithoutCancel and bounded by Agent.ShutdownGrace, so cancellation can neither abandon
// an event the runtime already accepted nor record a command that never reached the battery.
func Run(ctx context.Context, agent Agent) error {
	if agent.Repository == nil {
		return fmt.Errorf("persistence repository must not be nil")
	}
	if strings.TrimSpace(agent.DeviceID) == "" {
		return fmt.Errorf("device ID must not be empty")
	}

	interval := agent.Policy.Interval()
	healthPolicy, err := household.NewHealthPolicy(interval, 0, 0)
	if err != nil {
		return fmt.Errorf("configure health policy: %w", err)
	}

	snapshot, _, err := agent.Repository.Snapshot(ctx, agent.DeviceID)
	if err != nil {
		return fmt.Errorf("restore device snapshot: %w", err)
	}
	state := snapshot.State
	health := snapshot.Health

	for count := 0; agent.MaxIntervals <= 0 || count < agent.MaxIntervals; count++ {
		// Check cancellation before the select rather than leaving it to the select's own
		// ctx.Done case: both cases can be ready at once, and select picks among ready cases at
		// random, so a cancelled run would otherwise sometimes take one more tick.
		if err := ctx.Err(); err != nil {
			return err
		}

		// Wait between observations, not ahead of the first one, so an agent reports its
		// opening interval immediately instead of after one silent interval.
		if count > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-agent.Clock.Tick(ctx, interval):
			}
		}

		newState, newHealth, exhausted, err := processInterval(ctx, agent, healthPolicy, interval, state, health)
		if exhausted {
			return nil
		}
		if err != nil {
			return err
		}
		state, health = newState, newHealth
	}

	return nil
}

// processInterval handles one interval: it opens the observer scope, gets the next observation,
// classifies it, commits and applies it, and writes the resulting Record — closing the scope
// with defer regardless of which of those steps ends the interval. exhausted reports that the
// source has no more observations, which ends the run cleanly rather than as a failure.
func processInterval(
	ctx context.Context,
	agent Agent,
	healthPolicy household.HealthPolicy,
	interval time.Duration,
	state household.State,
	health household.DeviceHealth,
) (newState household.State, newHealth household.DeviceHealth, exhausted bool, err error) {
	newState, newHealth = state, health

	observer := agent.Observer
	if observer == nil {
		observer = noopObserver{}
	}
	scopeCtx, end := observer.BeginInterval(ctx)

	var record Record
	defer func() { end(record, err) }()

	envelope, nextErr := agent.Source.Next(scopeCtx)
	if errors.Is(nextErr, ErrSourceExhausted) {
		return newState, newHealth, true, nil
	}
	if nextErr != nil {
		err = fmt.Errorf("get telemetry observation: %w", nextErr)
		return newState, newHealth, false, err
	}

	now := agent.Clock.Now()
	classification := household.Classify(household.ClassifyInput{
		Envelope:    envelope,
		PriorState:  state,
		PriorHealth: health,
		Policy:      healthPolicy,
		Interval:    interval,
		Now:         now,
	})

	var command *household.Command
	if classification.Disposition == household.DispositionAccepted && !classification.SuppressCommand {
		decided := agent.Policy.Decide(*classification.State)
		command = &decided
	}

	receivedAt := now
	if envelope != nil && !envelope.ReceivedAt.IsZero() && envelope.ReceivedAt.Location() == time.UTC {
		receivedAt = envelope.ReceivedAt
	}

	observationResult := buildObservationResult(agent.DeviceID, receivedAt, classification, command)

	// Commit and apply on a context cancellation cannot reach, so an observation the runtime
	// already classified is neither abandoned before it is durable nor recorded as commanded
	// without the command reaching the battery. The grace bounds both, and derives from the
	// scope context so anything the observer attached — a trace span, for instance — survives
	// into the grace window.
	graceCtx, endGrace := context.WithTimeout(context.WithoutCancel(scopeCtx), agent.ShutdownGrace)
	defer endGrace()

	status, err := agent.Repository.CommitProcessing(graceCtx, observationResult)
	if err != nil {
		err = fmt.Errorf("commit processing result within the %s shutdown grace: %w", agent.ShutdownGrace, err)
		return newState, newHealth, false, err
	}

	result := outcome{
		disposition:   classification.Disposition,
		reason:        classification.Reason,
		telemetry:     classification.Telemetry,
		stateUpdated:  classification.State != nil,
		storedHistory: classification.Telemetry != nil,
		health:        classification.Health,
		command:       command,
	}

	if status == persistence.CommitDuplicate {
		// The event still gets reported with its telemetry, but nothing about it became durable
		// this time round, so no durable record changed — health included.
		result.disposition = household.DispositionDuplicate
		result.reason = "event ID was already processed"
		result.stateUpdated = false
		result.storedHistory = false
		result.health = health
		result.command = nil
	} else {
		if classification.State != nil {
			newState = *classification.State
		}
		newHealth = classification.Health
	}

	if err = agent.Sink.Apply(graceCtx, result.command); err != nil {
		err = fmt.Errorf("apply control command: %w", err)
		return newState, newHealth, false, err
	}

	record = buildRecord(agent, receivedAt, result)
	if err = agent.Write(record); err != nil {
		err = fmt.Errorf("write simulation record: %w", err)
		return newState, newHealth, false, err
	}

	return newState, newHealth, false, nil
}

// outcome is one interval's reportable result, after the commit settled whether the event was
// new or a duplicate the runtime had already durably processed.
type outcome struct {
	disposition   household.Disposition
	reason        string
	telemetry     *household.Telemetry
	stateUpdated  bool
	storedHistory bool
	health        household.DeviceHealth
	command       *household.Command
}

func buildObservationResult(
	deviceID string,
	receivedAt time.Time,
	classification household.ClassifyResult,
	command *household.Command,
) persistence.ObservationResult {
	result := persistence.ObservationResult{DeviceID: deviceID, Health: classification.Health}

	if classification.Telemetry == nil {
		return result
	}

	result.Telemetry = &persistence.TelemetryRecord{
		Event:             *classification.Telemetry,
		ReceivedAt:        receivedAt,
		Disposition:       classification.Disposition,
		DispositionReason: classification.Reason,
	}
	if classification.State != nil {
		result.LatestState = classification.State
	}
	if command != nil {
		result.Command = &persistence.CommandRecord{
			EventID:   classification.Telemetry.EventID,
			Command:   *command,
			CreatedAt: receivedAt,
		}
	}

	return result
}

func buildRecord(agent Agent, receivedAt time.Time, result outcome) Record {
	record := Record{
		AgentID:            agent.ID,
		DeviceID:           agent.DeviceID,
		ReceivedAt:         receivedAt,
		Disposition:        result.disposition,
		DispositionReason:  result.reason,
		StoredHistory:      result.storedHistory,
		StateUpdated:       result.stateUpdated,
		HealthStatus:       result.health.Status,
		HealthReason:       result.health.Reason,
		HealthTransitionAt: result.health.TransitionTime,
	}

	if result.telemetry != nil {
		record.EventID = result.telemetry.EventID
		eventTime := result.telemetry.EventTime
		record.Timestamp = &eventTime
		pv, load := result.telemetry.PVPowerKW, result.telemetry.LoadPowerKW
		soc, price := result.telemetry.BatterySOCPercent, result.telemetry.PriceEURPerKWh
		record.PVPowerKW = &pv
		record.LoadPowerKW = &load
		record.BatterySOCPercent = &soc
		record.PriceEURPerKWh = &price
	}

	if result.command != nil {
		record.Decision = result.command.Decision
		powerKW := result.command.PowerKW
		record.CommandPowerKW = &powerKW
		record.Reason = result.command.Reason
	}

	return record
}
