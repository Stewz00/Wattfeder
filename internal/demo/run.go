package demo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/Stewz00/wattfeder/internal/application"
	"github.com/Stewz00/wattfeder/internal/household"
	"github.com/Stewz00/wattfeder/internal/persistence/sqlite"
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
	Disposition       string  `json:"disposition"`
	DispositionReason string  `json:"disposition_reason,omitempty"`
	StateUpdated      bool    `json:"state_updated"`
	HealthStatus      string  `json:"health_status"`
	HealthReason      string  `json:"health_reason,omitempty"`
}

// observationLog reports one interval that produced no telemetry to store: a rejected,
// missing, or unavailable observation.
type observationLog struct {
	Event             string `json:"event"`
	DeviceID          string `json:"device_id"`
	ReceivedAt        string `json:"received_at"`
	Disposition       string `json:"disposition"`
	DispositionReason string `json:"disposition_reason,omitempty"`
	HealthStatus      string `json:"health_status"`
	HealthReason      string `json:"health_reason,omitempty"`
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

// demoShutdownGrace bounds each commit's context. A demo run is never cancelled mid-flight, so
// this only needs to be generous enough that a normal commit never approaches it.
const demoShutdownGrace = 5 * time.Second

// Run executes one scenario and writes structured progress records. It processes the
// scenario through the full durable flow, using a file-free in-memory SQLite database.
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

	repository, err := sqlite.Open(":memory:")
	if err != nil {
		return fmt.Errorf("open in-memory persistence: %w", err)
	}
	defer repository.Close()
	if err := repository.Migrate(ctx); err != nil {
		return fmt.Errorf("migrate in-memory persistence: %w", err)
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
	dispositions := make([]household.Disposition, 0, sim.IntervalsPerDay())
	healthStatuses := make([]household.DeviceHealthStatus, 0, sim.IntervalsPerDay())
	counts := make(map[household.Decision]int)
	// A scenario names a household, not an installed agent, so the runtime gets no agent
	// identity and the demo's own log lines carry none either.
	err = application.Run(ctx, application.Agent{
		Clock:         application.NewInstantClock(scenario.Config.Start.UTC()),
		Source:        sim,
		Sink:          sim,
		Policy:        policy,
		Repository:    repository,
		DeviceID:      scenario.Config.DeviceID,
		MaxIntervals:  sim.IntervalsPerDay(),
		ShutdownGrace: demoShutdownGrace,
		Write: func(record application.Record) error {
			if record.Timestamp != nil {
				if err := encoder.Encode(telemetryLog{
					Event:             "telemetry_produced",
					EventID:           string(record.EventID),
					Timestamp:         record.Timestamp.Format(time.RFC3339),
					DeviceID:          record.DeviceID,
					PVPowerKW:         *record.PVPowerKW,
					LoadPowerKW:       *record.LoadPowerKW,
					BatterySOCPercent: *record.BatterySOCPercent,
					PriceEURPerKWh:    *record.PriceEURPerKWh,
					Disposition:       string(record.Disposition),
					DispositionReason: record.DispositionReason,
					StateUpdated:      record.StateUpdated,
					HealthStatus:      string(record.HealthStatus),
					HealthReason:      record.HealthReason,
				}); err != nil {
					return err
				}
			} else {
				if err := encoder.Encode(observationLog{
					Event:             "observation_ignored",
					DeviceID:          record.DeviceID,
					ReceivedAt:        record.ReceivedAt.Format(time.RFC3339),
					Disposition:       string(record.Disposition),
					DispositionReason: record.DispositionReason,
					HealthStatus:      string(record.HealthStatus),
					HealthReason:      record.HealthReason,
				}); err != nil {
					return err
				}
			}

			if record.Decision != "" {
				if err := encoder.Encode(decisionLog{
					Event:          "decision_produced",
					EventID:        string(record.EventID),
					Timestamp:      record.Timestamp.Format(time.RFC3339),
					Decision:       record.Decision,
					CommandPowerKW: *record.CommandPowerKW,
					Reason:         record.Reason,
				}); err != nil {
					return err
				}
				counts[record.Decision]++
			}

			decisions = append(decisions, record.Decision)
			dispositions = append(dispositions, record.Disposition)
			healthStatuses = append(healthStatuses, record.HealthStatus)
			return nil
		},
	})
	if err != nil {
		return fmt.Errorf("run demo simulation: %w", err)
	}

	for i, decision := range decisions {
		if decision != scenario.ExpectedDecisions[i] {
			return fmt.Errorf("decision %d = %q, expected %q", i, decision, scenario.ExpectedDecisions[i])
		}
	}
	for i, disposition := range dispositions {
		if len(scenario.ExpectedDispositions) == 0 {
			break
		}
		if disposition != scenario.ExpectedDispositions[i] {
			return fmt.Errorf("disposition %d = %q, expected %q", i, disposition, scenario.ExpectedDispositions[i])
		}
	}
	for i, status := range healthStatuses {
		if len(scenario.ExpectedHealthStatuses) == 0 {
			break
		}
		if status != scenario.ExpectedHealthStatuses[i] {
			return fmt.Errorf("health status %d = %q, expected %q", i, status, scenario.ExpectedHealthStatuses[i])
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
