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

// Scenario contains one fixed simulator configuration and its expected decisions.
type Scenario struct {
	Name              string
	Duration          time.Duration
	Config            simulator.Config
	ExpectedDecisions []household.Decision
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
	Expected struct {
		Decisions []household.Decision `json:"decisions"`
	} `json:"expected"`
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
		},
		ExpectedDecisions: document.Expected.Decisions,
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

	wantDecisionCount := int(s.Duration / s.Config.Interval)
	if len(s.ExpectedDecisions) != wantDecisionCount {
		return fmt.Errorf(
			"expected.decisions must contain %d entries, got %d",
			wantDecisionCount,
			len(s.ExpectedDecisions),
		)
	}
	for i, decision := range s.ExpectedDecisions {
		switch decision {
		case household.DecisionCharge, household.DecisionDischarge, household.DecisionIdle:
		default:
			return fmt.Errorf("expected.decisions[%d] has invalid decision %q", i, decision)
		}
	}

	return nil
}
