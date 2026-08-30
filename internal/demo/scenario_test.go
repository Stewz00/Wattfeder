package demo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

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

func TestParseScenarioParsesFaultSchedule(t *testing.T) {
	input := strings.Replace(validScenarioJSON, `"expected"`, `
  "faults": [
    {"step": 2, "kind": "missing_heartbeat"},
    {"step": 3, "kind": "delay", "delay": "45m"},
    {"step": 4, "kind": "out_of_order", "event_time_offset": "-30m", "event_id": "fault-ooo-1"},
    {"step": 5, "kind": "invalid_measurement", "measurement": "pv_power_kw", "value": -1}
  ],
  "expected"`, 1)

	scenario, err := ParseScenario(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseScenario() error = %v", err)
	}

	if len(scenario.Config.Faults) != 4 {
		t.Fatalf("fault count = %d, want 4", len(scenario.Config.Faults))
	}
	if scenario.Config.Faults[1].Kind != simulator.FaultDelay || scenario.Config.Faults[1].Delay != 45*time.Minute {
		t.Errorf("fault 1 = %+v, want a 45m delay fault", scenario.Config.Faults[1])
	}
	if scenario.Config.Faults[2].EventTimeOffset != -30*time.Minute || scenario.Config.Faults[2].EventID != "fault-ooo-1" {
		t.Errorf("fault 2 = %+v, want a -30m out_of_order fault with event ID fault-ooo-1", scenario.Config.Faults[2])
	}
	if scenario.Config.Faults[3].Measurement != simulator.MeasurementPVPower || scenario.Config.Faults[3].Value != -1 {
		t.Errorf("fault 3 = %+v, want an invalid pv_power_kw measurement of -1", scenario.Config.Faults[3])
	}
}

func TestParseScenarioRejectsInvalidFault(t *testing.T) {
	input := strings.Replace(validScenarioJSON, `"expected"`, `
  "faults": [{"step": 0, "kind": "missing_heartbeat"}],
  "expected"`, 1)

	_, err := ParseScenario(strings.NewReader(input))
	if err == nil || !strings.Contains(err.Error(), "step must be at least 1") {
		t.Errorf("ParseScenario() error = %v, want a fault validation error", err)
	}
}

func TestParseScenarioRejectsUnparsableFaultDuration(t *testing.T) {
	input := strings.Replace(validScenarioJSON, `"expected"`, `
  "faults": [{"step": 2, "kind": "delay", "delay": "not-a-duration"}],
  "expected"`, 1)

	_, err := ParseScenario(strings.NewReader(input))
	if err == nil || !strings.Contains(err.Error(), "delay") {
		t.Errorf("ParseScenario() error = %v, want a delay parse error", err)
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
		{name: "missing battery capacity_kwh", old: `"capacity_kwh": 10, `, wantErr: "battery.capacity_kwh is required"},
		{
			name:    "missing battery starting_soc_percent",
			old:     `, "starting_soc_percent": 50`,
			wantErr: "battery.starting_soc_percent is required",
		},
		{name: "missing pv peak_power_kw", old: `"peak_power_kw": 6`, wantErr: "pv.peak_power_kw is required"},
		{name: "missing load base_power_kw", old: `"base_power_kw": 0.4`, wantErr: "load.base_power_kw is required"},
		{
			name:    "missing price base_eur_per_kwh",
			old:     `"base_eur_per_kwh": 0.30`,
			wantErr: "price.base_eur_per_kwh is required",
		},
		{
			name:    "unparsable start",
			old:     `"start": "2026-01-01T00:00:00Z",`,
			new:     `"start": "not-a-time",`,
			wantErr: "parse start",
		},
		{
			name:    "unparsable duration",
			old:     `"duration": "24h"`,
			new:     `"duration": "not-a-duration"`,
			wantErr: "parse duration",
		},
		{
			name:    "unparsable interval",
			old:     `"interval": "24h"`,
			new:     `"interval": "not-a-duration"`,
			wantErr: "parse interval",
		},
		{name: "empty name", old: `"name": "test-day",`, new: `"name": "",`, wantErr: "name must not be empty"},
		{
			name:    "invalid expected decision",
			old:     `"discharge"`,
			new:     `"invalid-decision"`,
			wantErr: "has invalid decision",
		},
		{
			name:    "wrong expected dispositions count",
			old:     `"expected": {"decisions": ["discharge"]}`,
			new:     `"expected": {"decisions": ["discharge"], "dispositions": ["accepted", "accepted"]}`,
			wantErr: "expected.dispositions must contain 1 entries",
		},
		{
			name:    "invalid expected disposition",
			old:     `"expected": {"decisions": ["discharge"]}`,
			new:     `"expected": {"decisions": ["discharge"], "dispositions": ["bogus"]}`,
			wantErr: "has invalid disposition",
		},
		{
			name:    "wrong expected health_statuses count",
			old:     `"expected": {"decisions": ["discharge"]}`,
			new:     `"expected": {"decisions": ["discharge"], "health_statuses": ["online", "online"]}`,
			wantErr: "expected.health_statuses must contain 1 entries",
		},
		{
			name:    "invalid expected health status",
			old:     `"expected": {"decisions": ["discharge"]}`,
			new:     `"expected": {"decisions": ["discharge"], "health_statuses": ["bogus"]}`,
			wantErr: "has invalid health status",
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

func TestParseScenarioRejectsTrailingContent(t *testing.T) {
	tests := []struct {
		name    string
		extra   string
		wantErr string
	}{
		{name: "second JSON value", extra: "\n{}", wantErr: "multiple values are not allowed"},
		{name: "malformed trailing content", extra: "\nnot-json", wantErr: "decode JSON"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validScenarioJSON + tt.extra
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
	telemetryEventID := fmt.Sprint(logs[2]["event_id"])
	decisionEventID := fmt.Sprint(logs[3]["event_id"])
	if telemetryEventID == "" || decisionEventID != telemetryEventID {
		t.Errorf(
			"telemetry event ID = %q, decision event ID = %q, want one shared non-empty ID",
			telemetryEventID,
			decisionEventID,
		)
	}
}

// The fixture always discharges, so "charge" is a guaranteed mismatch.
func TestRunReturnsErrorOnDecisionMismatch(t *testing.T) {
	input := strings.Replace(validScenarioJSON, `"discharge"`, `"charge"`, 1)
	scenario, err := ParseScenario(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseScenario() error = %v", err)
	}

	err = Run(context.Background(), scenario, io.Discard)
	if err == nil {
		t.Fatal("Run() error = nil, want a decision mismatch error")
	}
	wantErr := `decision 0 = "discharge", expected "charge"`
	if !strings.Contains(err.Error(), wantErr) {
		t.Errorf("Run() error = %q, want it to contain %q", err.Error(), wantErr)
	}
}

// The fixture's telemetry is well-formed, so it is accepted and "rejected" cannot match.
func TestRunReturnsErrorOnDispositionMismatch(t *testing.T) {
	input := strings.Replace(
		validScenarioJSON,
		`"expected": {"decisions": ["discharge"]}`,
		`"expected": {"decisions": ["discharge"], "dispositions": ["rejected"]}`,
		1,
	)
	scenario, err := ParseScenario(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseScenario() error = %v", err)
	}

	err = Run(context.Background(), scenario, io.Discard)
	if err == nil {
		t.Fatal("Run() error = nil, want a disposition mismatch error")
	}
	wantErr := `disposition 0 = "accepted", expected "rejected"`
	if !strings.Contains(err.Error(), wantErr) {
		t.Errorf("Run() error = %q, want it to contain %q", err.Error(), wantErr)
	}
}

// The fixture's device reports fresh telemetry, so it is online and "offline" cannot match.
func TestRunReturnsErrorOnHealthStatusMismatch(t *testing.T) {
	input := strings.Replace(
		validScenarioJSON,
		`"expected": {"decisions": ["discharge"]}`,
		`"expected": {"decisions": ["discharge"], "health_statuses": ["offline"]}`,
		1,
	)
	scenario, err := ParseScenario(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseScenario() error = %v", err)
	}

	err = Run(context.Background(), scenario, io.Discard)
	if err == nil {
		t.Fatal("Run() error = nil, want a health status mismatch error")
	}
	wantErr := `health status 0 = "online", expected "offline"`
	if !strings.Contains(err.Error(), wantErr) {
		t.Errorf("Run() error = %q, want it to contain %q", err.Error(), wantErr)
	}
}
