// Package application runs the single-household simulation data flow.
package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Stewz00/wattfeder/internal/household"
	"github.com/Stewz00/wattfeder/internal/persistence"
)

// Record is the additive flat outcome of processing one simulated interval, including
// intervals that produced no telemetry or command. Timestamp, the measurement fields, and
// CommandPowerKW are nil precisely when the corresponding data does not exist for this
// interval's disposition; Decision, DispositionReason, HealthReason, and Reason are empty
// strings under the same condition.
type Record struct {
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

type simulation interface {
	IntervalsPerDay() int
	NextObservation() (*household.ObservationEnvelope, time.Time, error)
	Complete(*household.Command) error
}

// RunPersistentDay processes one simulated day, classifying and durably committing every
// interval's observation before deciding whether to apply its command. It continues past
// duplicate, historical, delayed, incomplete, invalid, missing-heartbeat, and unavailable
// observations, writing one Record per interval regardless of disposition. Only context
// cancellation, a simulator error, a persistence error, or a write error stop it early.
func RunPersistentDay(
	ctx context.Context,
	sim simulation,
	policy household.Policy,
	repository persistence.Repository,
	deviceID string,
	write func(Record) error,
) error {
	if repository == nil {
		return fmt.Errorf("persistence repository must not be nil")
	}
	if strings.TrimSpace(deviceID) == "" {
		return fmt.Errorf("device ID must not be empty")
	}

	healthPolicy, err := household.NewHealthPolicy(policy.Interval(), 0, 0)
	if err != nil {
		return fmt.Errorf("configure health policy: %w", err)
	}

	snapshot, _, err := repository.Snapshot(ctx, deviceID)
	if err != nil {
		return fmt.Errorf("restore device snapshot: %w", err)
	}
	state := snapshot.State
	health := snapshot.Health

	for range sim.IntervalsPerDay() {
		if err := ctx.Err(); err != nil {
			return err
		}

		envelope, nominalTime, err := sim.NextObservation()
		if err != nil {
			return fmt.Errorf("get simulated observation: %w", err)
		}

		classification := household.Classify(household.ClassifyInput{
			Envelope:    envelope,
			PriorState:  state,
			PriorHealth: health,
			Policy:      healthPolicy,
			Interval:    policy.Interval(),
			Now:         nominalTime,
		})

		var command *household.Command
		if classification.Disposition == household.DispositionAccepted && !classification.SuppressCommand {
			decided := policy.Decide(*classification.State)
			command = &decided
		}

		receivedAt := nominalTime
		if envelope != nil {
			receivedAt = envelope.ReceivedAt
		}

		observationResult := buildObservationResult(deviceID, receivedAt, classification, command)
		status, err := repository.CommitProcessing(ctx, observationResult)
		if err != nil {
			return fmt.Errorf("commit processing result: %w", err)
		}

		disposition := classification.Disposition
		reason := classification.Reason
		telemetry := classification.Telemetry
		stateUpdated := classification.State != nil
		storedHistory := classification.Telemetry != nil
		finalHealth := classification.Health

		if status == persistence.CommitDuplicate {
			disposition = household.DispositionDuplicate
			reason = "event ID was already processed"
			stateUpdated = false
			storedHistory = false
			finalHealth = health // a duplicate commit changes no durable record, including health
			command = nil
		} else {
			if classification.State != nil {
				state = *classification.State
			}
			health = classification.Health
		}

		if err := sim.Complete(command); err != nil {
			return fmt.Errorf("apply control command: %w", err)
		}

		record := buildRecord(deviceID, receivedAt, disposition, reason, telemetry, stateUpdated, storedHistory, finalHealth, command)
		if err := write(record); err != nil {
			return fmt.Errorf("write simulation record: %w", err)
		}
	}

	return nil
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

func buildRecord(
	deviceID string,
	receivedAt time.Time,
	disposition household.Disposition,
	reason string,
	telemetry *household.Telemetry,
	stateUpdated bool,
	storedHistory bool,
	health household.DeviceHealth,
	command *household.Command,
) Record {
	record := Record{
		DeviceID:           deviceID,
		ReceivedAt:         receivedAt,
		Disposition:        disposition,
		DispositionReason:  reason,
		StoredHistory:      storedHistory,
		StateUpdated:       stateUpdated,
		HealthStatus:       health.Status,
		HealthReason:       health.Reason,
		HealthTransitionAt: health.TransitionTime,
	}

	if telemetry != nil {
		record.EventID = telemetry.EventID
		eventTime := telemetry.EventTime
		record.Timestamp = &eventTime
		pv, load, soc, price := telemetry.PVPowerKW, telemetry.LoadPowerKW, telemetry.BatterySOCPercent, telemetry.PriceEURPerKWh
		record.PVPowerKW = &pv
		record.LoadPowerKW = &load
		record.BatterySOCPercent = &soc
		record.PriceEURPerKWh = &price
	}

	if command != nil {
		record.Decision = command.Decision
		powerKW := command.PowerKW
		record.CommandPowerKW = &powerKW
		record.Reason = command.Reason
	}

	return record
}
