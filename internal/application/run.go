// Package application runs the single-household simulation data flow.
package application

import (
	"context"
	"fmt"
	"time"

	"github.com/Stewz00/wattfeder/internal/household"
	"github.com/Stewz00/wattfeder/internal/simulator"
)

// Record is the telemetry event and resulting command for one simulation interval.
type Record struct {
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

// RunDay processes one simulated day and writes one record for each telemetry event.
func RunDay(ctx context.Context, sim *simulator.Simulator, write func(Record) error) error {
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

		command := household.Decide(state)
		if err := sim.ApplyCommand(command); err != nil {
			return fmt.Errorf("apply control command: %w", err)
		}

		record := Record{
			Timestamp:         event.Timestamp,
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
