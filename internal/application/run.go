// Package application runs the single-household simulation data flow.
package application

import (
	"context"
	"fmt"
	"time"

	"github.com/Stewz00/wattfeder/internal/household"
	"github.com/Stewz00/wattfeder/internal/persistence"
)

// Record is the telemetry event and resulting command for one simulation interval.
type Record struct {
	EventID           household.EventID  `json:"event_id"`
	Timestamp         time.Time          `json:"timestamp"`
	DeviceID          string             `json:"device_id"`
	PVPowerKW         float64            `json:"pv_power_kw"`
	LoadPowerKW       float64            `json:"load_power_kw"`
	BatterySOCPercent float64            `json:"battery_soc_percent"`
	PriceEURPerKWh    float64            `json:"electricity_price_eur_kwh"`
	Decision          household.Decision `json:"decision"`
	CommandPowerKW    float64            `json:"command_power_kw"`
	Reason            string             `json:"reason"`
}

type simulation interface {
	IntervalsPerDay() int
	NextTelemetry() (household.Telemetry, error)
	ApplyCommand(household.Command) error
}

// RunDay processes one simulated day and writes one record for each telemetry event.
func RunDay(ctx context.Context, sim simulation, policy household.Policy, write func(Record) error) error {
	return runDay(ctx, sim, policy, nil, write)
}

// RunPersistentDay processes one simulated day and commits each result before applying its command.
func RunPersistentDay(
	ctx context.Context,
	sim simulation,
	policy household.Policy,
	repository persistence.Repository,
	write func(Record) error,
) error {
	if repository == nil {
		return fmt.Errorf("persistence repository must not be nil")
	}

	return runDay(ctx, sim, policy, repository, write)
}

func runDay(
	ctx context.Context,
	sim simulation,
	policy household.Policy,
	repository persistence.Repository,
	write func(Record) error,
) error {
	var state household.State

	for range sim.IntervalsPerDay() {
		if err := ctx.Err(); err != nil {
			return err
		}

		event, err := sim.NextTelemetry()
		if err != nil {
			return fmt.Errorf("get simulated telemetry: %w", err)
		}
		if err := state.ApplyTelemetry(event); err != nil {
			return fmt.Errorf("apply telemetry: %w", err)
		}

		command := policy.Decide(state)
		if repository != nil {
			now := time.Now().UTC()
			committedState := state
			status, err := repository.CommitProcessing(ctx, persistence.ObservationResult{
				DeviceID: event.DeviceID,
				Telemetry: &persistence.TelemetryRecord{
					Event:             event,
					ReceivedAt:        now,
					Disposition:       household.DispositionAccepted,
					DispositionReason: "",
				},
				LatestState: &committedState,
				Command: &persistence.CommandRecord{
					EventID:   event.EventID,
					Command:   command,
					CreatedAt: now,
				},
				Health: household.DeviceHealth{
					Status:         household.HealthOnline,
					TransitionTime: now,
					LastContactAt:  now,
				},
			})
			if err != nil {
				return fmt.Errorf("commit processing result: %w", err)
			}
			switch status {
			case persistence.CommitStored:
			case persistence.CommitDuplicate:
				return nil
			default:
				return fmt.Errorf("commit processing result: unexpected status %d", status)
			}
		}
		if err := sim.ApplyCommand(command); err != nil {
			return fmt.Errorf("apply control command: %w", err)
		}

		record := Record{
			EventID:           event.EventID,
			Timestamp:         event.EventTime,
			DeviceID:          event.DeviceID,
			PVPowerKW:         event.PVPowerKW,
			LoadPowerKW:       event.LoadPowerKW,
			BatterySOCPercent: event.BatterySOCPercent,
			PriceEURPerKWh:    event.PriceEURPerKWh,
			Decision:          command.Decision,
			CommandPowerKW:    command.PowerKW,
			Reason:            command.Reason,
		}
		if err := write(record); err != nil {
			return fmt.Errorf("write simulation record: %w", err)
		}
	}

	return nil
}
