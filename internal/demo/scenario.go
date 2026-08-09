// Package demo loads and runs fixed scenarios through the Wattfeder application.
package demo

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Stewz00/wattfeder/internal/household"
	"github.com/Stewz00/wattfeder/internal/simulator"
)

// Scenario contains one fixed simulator configuration and its expected outcome.
// ExpectedDecisions must contain one entry per interval; an empty household.Decision means
// no command is expected that interval. ExpectedDispositions and ExpectedHealthStatuses are
// optional and, when non-empty, must also contain one entry per interval.
type Scenario struct {
	Name                   string
	Duration               time.Duration
	Config                 simulator.Config
	ExpectedDecisions      []household.Decision
	ExpectedDispositions   []household.Disposition
	ExpectedHealthStatuses []household.DeviceHealthStatus
}

type scenarioDocument struct {
	Name     string `json:"name"`
	Seed     *int64 `json:"seed"`
	Start    string `json:"start"`
	Duration string `json:"duration"`
	Interval string `json:"interval"`
	DeviceID string `json:"device_id"`
	Battery  struct {
		CapacityKWh        *float64 `json:"capacity_kwh"`
		StartingSOCPercent *float64 `json:"starting_soc_percent"`
	} `json:"battery"`
	PV struct {
		PeakPowerKW *float64 `json:"peak_power_kw"`
	} `json:"pv"`
	Load struct {
		BasePowerKW *float64 `json:"base_power_kw"`
	} `json:"load"`
	Price struct {
		BaseEURPerKWh *float64 `json:"base_eur_per_kwh"`
	} `json:"price"`
	Faults   []faultDocument `json:"faults"`
	Expected struct {
		Decisions      []household.Decision           `json:"decisions"`
		Dispositions   []household.Disposition        `json:"dispositions"`
		HealthStatuses []household.DeviceHealthStatus `json:"health_statuses"`
	} `json:"expected"`
}

type faultDocument struct {
	Step            int      `json:"step"`
	Repeat          int      `json:"repeat"`
	Kind            string   `json:"kind"`
	EventTimeOffset string   `json:"event_time_offset"`
	EventID         string   `json:"event_id"`
	Delay           string   `json:"delay"`
	Measurement     string   `json:"measurement"`
	Value           *float64 `json:"value"`
}

func (d faultDocument) toFault() (simulator.Fault, error) {
	fault := simulator.Fault{
		Step:        d.Step,
		Repeat:      d.Repeat,
		Kind:        simulator.FaultKind(d.Kind),
		EventID:     d.EventID,
		Measurement: simulator.Measurement(d.Measurement),
	}
	if d.Value != nil {
		fault.Value = *d.Value
	}

	if d.EventTimeOffset != "" {
		offset, err := time.ParseDuration(d.EventTimeOffset)
		if err != nil {
			return simulator.Fault{}, fmt.Errorf("parse fault event_time_offset: %w", err)
		}
		fault.EventTimeOffset = offset
	}
	if d.Delay != "" {
		delay, err := time.ParseDuration(d.Delay)
		if err != nil {
			return simulator.Fault{}, fmt.Errorf("parse fault delay: %w", err)
		}
		fault.Delay = delay
	}

	return fault, nil
}

// LoadScenario reads and validates a scenario from path.
func LoadScenario(path string) (Scenario, error) {
	file, err := os.Open(path)
	if err != nil {
		return Scenario{}, fmt.Errorf("open scenario: %w", err)
	}
	defer file.Close()

	scenario, err := ParseScenario(file)
	if err != nil {
		return Scenario{}, fmt.Errorf("parse scenario: %w", err)
	}

	return scenario, nil
}

// ParseScenario decodes and validates one JSON scenario.
func ParseScenario(input io.Reader) (Scenario, error) {
	var document scenarioDocument
	decoder := json.NewDecoder(input)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return Scenario{}, fmt.Errorf("decode JSON: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return Scenario{}, errors.New("decode JSON: multiple values are not allowed")
		}
		return Scenario{}, fmt.Errorf("decode JSON: %w", err)
	}

	if document.Seed == nil {
		return Scenario{}, errors.New("seed is required")
	}
	if document.Battery.CapacityKWh == nil {
		return Scenario{}, errors.New("battery.capacity_kwh is required")
	}
	if document.Battery.StartingSOCPercent == nil {
		return Scenario{}, errors.New("battery.starting_soc_percent is required")
	}
	if document.PV.PeakPowerKW == nil {
		return Scenario{}, errors.New("pv.peak_power_kw is required")
	}
	if document.Load.BasePowerKW == nil {
		return Scenario{}, errors.New("load.base_power_kw is required")
	}
	if document.Price.BaseEURPerKWh == nil {
		return Scenario{}, errors.New("price.base_eur_per_kwh is required")
	}

	start, err := time.Parse(time.RFC3339, document.Start)
	if err != nil {
		return Scenario{}, fmt.Errorf("parse start: %w", err)
	}
	duration, err := time.ParseDuration(document.Duration)
	if err != nil {
		return Scenario{}, fmt.Errorf("parse duration: %w", err)
	}
	interval, err := time.ParseDuration(document.Interval)
	if err != nil {
		return Scenario{}, fmt.Errorf("parse interval: %w", err)
	}

	faults := make(simulator.FaultSchedule, len(document.Faults))
	for i, faultDoc := range document.Faults {
		fault, err := faultDoc.toFault()
		if err != nil {
			return Scenario{}, fmt.Errorf("parse faults[%d]: %w", i, err)
		}
		faults[i] = fault
	}

	scenario := Scenario{
		Name:     document.Name,
		Duration: duration,
		Config: simulator.Config{
			Seed:                      *document.Seed,
			Start:                     start,
			Interval:                  interval,
			DeviceID:                  document.DeviceID,
			BatteryCapacityKWh:        *document.Battery.CapacityKWh,
			StartingBatterySOCPercent: *document.Battery.StartingSOCPercent,
			PVPeakPowerKW:             *document.PV.PeakPowerKW,
			LoadBasePowerKW:           *document.Load.BasePowerKW,
			PriceBaseEURPerKWh:        *document.Price.BaseEURPerKWh,
			Faults:                    faults,
		},
		ExpectedDecisions:      document.Expected.Decisions,
		ExpectedDispositions:   document.Expected.Dispositions,
		ExpectedHealthStatuses: document.Expected.HealthStatuses,
	}
	if err := scenario.Validate(); err != nil {
		return Scenario{}, err
	}

	return scenario, nil
}

// Validate checks that a scenario is complete and matches the simulator's fixed timeline.
func (s Scenario) Validate() error {
	if strings.TrimSpace(s.Name) == "" {
		return errors.New("name must not be empty")
	}
	if s.Duration != simulator.SimulationDuration {
		return fmt.Errorf("duration must be %s, got %s", simulator.SimulationDuration, s.Duration)
	}
	if err := s.Config.Validate(); err != nil {
		return fmt.Errorf("invalid simulator configuration: %w", err)
	}

	wantCount := int(s.Duration / s.Config.Interval)
	if len(s.ExpectedDecisions) != wantCount {
		return fmt.Errorf(
			"expected.decisions must contain %d entries, got %d",
			wantCount,
			len(s.ExpectedDecisions),
		)
	}
	for i, decision := range s.ExpectedDecisions {
		switch decision {
		case household.DecisionCharge, household.DecisionDischarge, household.DecisionIdle, "":
		default:
			return fmt.Errorf("expected.decisions[%d] has invalid decision %q", i, decision)
		}
	}

	if len(s.ExpectedDispositions) != 0 && len(s.ExpectedDispositions) != wantCount {
		return fmt.Errorf(
			"expected.dispositions must contain %d entries, got %d",
			wantCount,
			len(s.ExpectedDispositions),
		)
	}
	for i, disposition := range s.ExpectedDispositions {
		switch disposition {
		case household.DispositionAccepted, household.DispositionHistoryOnly, household.DispositionDuplicate,
			household.DispositionRejected, household.DispositionMissing, household.DispositionUnavailable:
		default:
			return fmt.Errorf("expected.dispositions[%d] has invalid disposition %q", i, disposition)
		}
	}

	if len(s.ExpectedHealthStatuses) != 0 && len(s.ExpectedHealthStatuses) != wantCount {
		return fmt.Errorf(
			"expected.health_statuses must contain %d entries, got %d",
			wantCount,
			len(s.ExpectedHealthStatuses),
		)
	}
	for i, status := range s.ExpectedHealthStatuses {
		switch status {
		case household.HealthOnline, household.HealthStale, household.HealthOffline, household.HealthInvalid:
		default:
			return fmt.Errorf("expected.health_statuses[%d] has invalid health status %q", i, status)
		}
	}

	return nil
}
