package demo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/Stewz00/wattfeder/internal/application"
	"github.com/Stewz00/wattfeder/internal/household"
	"github.com/Stewz00/wattfeder/internal/simulator"
)

type scenarioLoadedLog struct {
	Event      string `json:"event"`
	Scenario   string `json:"scenario"`
	Start      string `json:"start"`
	Duration   string `json:"duration"`
	Interval   string `json:"interval"`
	DeviceID   string `json:"device_id"`
	EventCount int    `json:"event_count"`
}

type simulationStartedLog struct {
	Event    string `json:"event"`
	Scenario string `json:"scenario"`
}

type telemetryLog struct {
	Event             string  `json:"event"`
	EventID           string  `json:"event_id"`
	Timestamp         string  `json:"timestamp"`
	DeviceID          string  `json:"device_id"`
	PVPowerKW         float64 `json:"pv_power_kw"`
	LoadPowerKW       float64 `json:"load_power_kw"`
	BatterySOCPercent float64 `json:"battery_soc_percent"`
	PriceEURPerKWh    float64 `json:"electricity_price_eur_kwh"`
}

type decisionLog struct {
	Event          string             `json:"event"`
	EventID        string             `json:"event_id"`
	Timestamp      string             `json:"timestamp"`
	Decision       household.Decision `json:"decision"`
	CommandPowerKW float64            `json:"command_power_kw"`
	Reason         string             `json:"reason"`
}

type simulationCompletedLog struct {
	Event              string `json:"event"`
	Scenario           string `json:"scenario"`
	Records            int    `json:"records"`
	ChargeDecisions    int    `json:"charge_decisions"`
	DischargeDecisions int    `json:"discharge_decisions"`
	IdleDecisions      int    `json:"idle_decisions"`
	ExpectedResult     string `json:"expected_result"`
}

// Run executes one scenario and writes structured progress records.
func Run(ctx context.Context, scenario Scenario, output io.Writer) error {
	if err := scenario.Validate(); err != nil {
		return fmt.Errorf("validate demo scenario: %w", err)
	}

	sim, err := simulator.New(scenario.Config)
	if err != nil {
		return err
	}
	policy, err := household.NewPolicy(scenario.Config.BatteryCapacityKWh, scenario.Config.Interval)
	if err != nil {
		return fmt.Errorf("configure control policy: %w", err)
	}

	encoder := json.NewEncoder(output)
	if err := encoder.Encode(scenarioLoadedLog{
		Event:      "demo_scenario_loaded",
		Scenario:   scenario.Name,
		Start:      scenario.Config.Start.UTC().Format(time.RFC3339),
		Duration:   scenario.Duration.String(),
		Interval:   scenario.Config.Interval.String(),
		DeviceID:   scenario.Config.DeviceID,
		EventCount: sim.IntervalsPerDay(),
	}); err != nil {
		return fmt.Errorf("write scenario log: %w", err)
	}
	if err := encoder.Encode(simulationStartedLog{Event: "simulation_started", Scenario: scenario.Name}); err != nil {
		return fmt.Errorf("write start log: %w", err)
	}

	decisions := make([]household.Decision, 0, sim.IntervalsPerDay())
	counts := make(map[household.Decision]int)
	err = application.RunDay(ctx, sim, policy, func(record application.Record) error {
		if err := encoder.Encode(telemetryLog{
			Event:             "telemetry_produced",
			EventID:           string(record.EventID),
			Timestamp:         record.Timestamp.Format(time.RFC3339),
			DeviceID:          record.DeviceID,
			PVPowerKW:         record.PVPowerKW,
			LoadPowerKW:       record.LoadPowerKW,
			BatterySOCPercent: record.BatterySOCPercent,
			PriceEURPerKWh:    record.PriceEURPerKWh,
		}); err != nil {
			return err
		}
		if err := encoder.Encode(decisionLog{
			Event:          "decision_produced",
			EventID:        string(record.EventID),
			Timestamp:      record.Timestamp.Format(time.RFC3339),
			Decision:       record.Decision,
			CommandPowerKW: record.CommandPowerKW,
			Reason:         record.Reason,
		}); err != nil {
			return err
		}

		decisions = append(decisions, record.Decision)
		counts[record.Decision]++
		return nil
	})
	if err != nil {
		return fmt.Errorf("run demo simulation: %w", err)
	}

	for i, decision := range decisions {
		if decision != scenario.ExpectedDecisions[i] {
			return fmt.Errorf(
				"decision %d = %q, expected %q",
				i,
				decision,
				scenario.ExpectedDecisions[i],
			)
		}
	}

	if err := encoder.Encode(simulationCompletedLog{
		Event:              "simulation_completed",
		Scenario:           scenario.Name,
		Records:            len(decisions),
		ChargeDecisions:    counts[household.DecisionCharge],
		DischargeDecisions: counts[household.DecisionDischarge],
		IdleDecisions:      counts[household.DecisionIdle],
		ExpectedResult:     "matched",
	}); err != nil {
		return fmt.Errorf("write completion log: %w", err)
	}

	return nil
}
