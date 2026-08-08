package demo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Stewz00/wattfeder/internal/household"
	"github.com/Stewz00/wattfeder/internal/simulator"
)

const validScenarioJSON = `{
  "name": "test-day",
  "seed": 42,
  "start": "2026-01-01T00:00:00Z",
  "duration": "24h",
  "interval": "24h",
  "device_id": "test-home-001",
  "battery": {"capacity_kwh": 10, "starting_soc_percent": 50},
  "pv": {"peak_power_kw": 6},
  "load": {"base_power_kw": 0.4},
  "price": {"base_eur_per_kwh": 0.30},
  "expected": {"decisions": ["discharge"]}
}`

func TestParseScenario(t *testing.T) {
	scenario, err := ParseScenario(strings.NewReader(validScenarioJSON))
	if err != nil {
		t.Fatalf("ParseScenario() error = %v", err)
	}

	if scenario.Name != "test-day" {
		t.Errorf("name = %q, want test-day", scenario.Name)
	}
	if scenario.Duration != simulator.SimulationDuration {
		t.Errorf("duration = %s, want %s", scenario.Duration, simulator.SimulationDuration)
	}
	if scenario.Config.Seed != 42 || scenario.Config.DeviceID != "test-home-001" {
		t.Errorf(
			"config seed = %d, device ID = %q, want 42 and test-home-001",
			scenario.Config.Seed,
			scenario.Config.DeviceID,
		)
	}
	if len(scenario.ExpectedDecisions) != 1 || scenario.ExpectedDecisions[0] != household.DecisionDischarge {
		t.Errorf("expected decisions = %v, want [discharge]", scenario.ExpectedDecisions)
	}
}

func TestParseScenarioRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name    string
		old     string
		new     string
		wantErr string
	}{
		{name: "missing seed", old: `"seed": 42,`, wantErr: "seed is required"},
		{name: "wrong duration", old: `"duration": "24h"`, new: `"duration": "12h"`, wantErr: "duration must be 24h0m0s"},
		{name: "invalid interval", old: `"interval": "24h"`, new: `"interval": "7m"`, wantErr: "must divide"},
		{name: "missing expected decision", old: `"discharge"`, wantErr: "must contain 1 entries"},
		{
			name:    "unknown field",
			old:     `"name": "test-day",`,
			new:     `"name": "test-day", "extra": true,`,
			wantErr: "unknown field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := strings.Replace(validScenarioJSON, tt.old, tt.new, 1)
			_, err := ParseScenario(strings.NewReader(input))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("ParseScenario() error = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestRunIsDeterministicAndMatchesExpectedResult(t *testing.T) {
	scenario, err := ParseScenario(strings.NewReader(validScenarioJSON))
	if err != nil {
		t.Fatalf("ParseScenario() error = %v", err)
	}

	var first bytes.Buffer
	if err := Run(context.Background(), scenario, &first); err != nil {
		t.Fatalf("first Run() error = %v", err)
	}
	var second bytes.Buffer
	if err := Run(context.Background(), scenario, &second); err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
	if first.String() != second.String() {
		t.Errorf("identical demo runs differ:\nfirst:\n%s\nsecond:\n%s", first.String(), second.String())
	}

	var logs []map[string]any
	decoder := json.NewDecoder(&first)
	for decoder.More() {
		var log map[string]any
		if err := decoder.Decode(&log); err != nil {
			t.Fatalf("decode demo log: %v", err)
		}
		logs = append(logs, log)
	}
	if len(logs) != 5 {
		t.Fatalf("log count = %d, want 5", len(logs))
	}
	completion := logs[len(logs)-1]
	if completion["event"] != "simulation_completed" || completion["records"] != float64(1) ||
		completion["expected_result"] != "matched" {
		t.Errorf("completion log = %v, want one record and matched expected result", completion)
	}
	if got := fmt.Sprint(logs[3]["decision"]); got != "discharge" {
		t.Errorf("decision = %q, want discharge", got)
	}
}
