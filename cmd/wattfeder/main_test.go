package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Stewz00/wattfeder/internal/application"
)

func TestRunEmitsJSONRecords(t *testing.T) {
	var output bytes.Buffer
	if err := runIsolated(t, context.Background(), []string{"-interval", "24h"}, &output); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	decoder := json.NewDecoder(&output)
	var record application.Record
	if err := decoder.Decode(&record); err != nil {
		t.Fatalf("decode output: %v", err)
	}

	if record.DeviceID != "home-001" {
		t.Errorf("device ID = %q, want %q", record.DeviceID, "home-001")
	}
	if record.EventID == "" {
		t.Error("event ID is empty")
	}
	if record.Decision == "" || record.Reason == "" {
		t.Errorf("decision = %q, reason = %q, want both populated", record.Decision, record.Reason)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		t.Errorf("decode after first record error = %v, want EOF", err)
	}
}

func TestRunRejectsInvalidStart(t *testing.T) {
	err := run(context.Background(), []string{"-start", "not-a-time"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "parse -start") {
		t.Errorf("run() error = %v, want start parsing error", err)
	}
}

func TestRunMapsConfigurationFlagsToOutput(t *testing.T) {
	var output bytes.Buffer
	args := []string{
		"-interval", "24h",
		"-start", "2026-08-08T06:30:00+02:00",
		"-device-id", "home-test",
		"-starting-battery-soc-percent", "73",
	}
	if err := runIsolated(t, context.Background(), args, &output); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	var record application.Record
	if err := json.NewDecoder(&output).Decode(&record); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	wantTimestamp := time.Date(2026, 8, 8, 4, 30, 0, 0, time.UTC)
	if !record.Timestamp.Equal(wantTimestamp) {
		t.Errorf("timestamp = %v, want %v", record.Timestamp, wantTimestamp)
	}
	if record.DeviceID != "home-test" {
		t.Errorf("device ID = %q, want %q", record.DeviceID, "home-test")
	}
	if record.BatterySOCPercent != 73 {
		t.Errorf("battery SOC = %v, want 73", record.BatterySOCPercent)
	}
}

func TestRunIsDeterministic(t *testing.T) {
	args := []string{"-interval", "24h", "-seed", "123"}
	var first bytes.Buffer
	var second bytes.Buffer

	if err := runIsolated(t, context.Background(), args, &first); err != nil {
		t.Fatalf("first run() error = %v", err)
	}
	if err := runIsolated(t, context.Background(), args, &second); err != nil {
		t.Fatalf("second run() error = %v", err)
	}
	if first.String() != second.String() {
		t.Errorf("identical runs produced different output:\nfirst:  %s\nsecond: %s", first.String(), second.String())
	}
}

func TestRunDemoScenario(t *testing.T) {
	scenarioPath := filepath.Join("..", "..", "scenarios", "demo.json")
	args := []string{"-scenario", scenarioPath}
	var first bytes.Buffer
	var second bytes.Buffer

	if err := run(context.Background(), args, &first); err != nil {
		t.Fatalf("first run() error = %v", err)
	}
	if err := run(context.Background(), args, &second); err != nil {
		t.Fatalf("second run() error = %v", err)
	}
	if first.String() != second.String() {
		t.Error("identical scenario runs produced different output")
	}

	output := first.String()
	for _, want := range []string{
		`"event":"demo_scenario_loaded"`,
		`"event":"telemetry_produced"`,
		`"event":"decision_produced"`,
		`"event":"simulation_completed"`,
		`"records":4`,
		`"charge_decisions":1`,
		`"discharge_decisions":3`,
		`"expected_result":"matched"`,
	} {
		if !strings.Contains(output, want) {
			t.Errorf("run() output does not contain %q:\n%s", want, output)
		}
	}
}

func TestRunRejectsScenarioWithConfigurationFlags(t *testing.T) {
	err := run(context.Background(), []string{"-scenario", "demo.json", "-seed", "1"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "cannot be combined") {
		t.Errorf("run() error = %v, want conflicting-flags error", err)
	}
}

func TestRunRejectsInvalidArguments(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "unknown flag", args: []string{"-unknown"}, wantErr: "flag provided but not defined"},
		{name: "missing flag value", args: []string{"-interval"}, wantErr: "flag needs an argument"},
		{name: "invalid duration", args: []string{"-interval", "tomorrow"}, wantErr: "invalid value"},
		{name: "partial final interval", args: []string{"-interval", "7m"}, wantErr: "must divide"},
		{name: "blank device ID", args: []string{"-device-id", "  "}, wantErr: "device ID"},
		{name: "zero battery capacity", args: []string{"-battery-capacity-kwh", "0"}, wantErr: "battery capacity"},
		{name: "blank database path", args: []string{"-database", " "}, wantErr: "SQLite path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := run(context.Background(), tt.args, &bytes.Buffer{})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("run() error = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestRunReturnsOutputError(t *testing.T) {
	wantErr := errors.New("output unavailable")
	err := runIsolated(t, context.Background(), []string{"-interval", "24h"}, errorWriter{err: wantErr})
	if !errors.Is(err, wantErr) {
		t.Errorf("run() error = %v, want wrapped output error %v", err, wantErr)
	}
}

func TestRunPrintsHelp(t *testing.T) {
	var output bytes.Buffer
	if err := run(context.Background(), []string{"-help"}, &output); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	help := output.String()
	if !strings.Contains(help, "Usage: wattfeder [options]") ||
		!strings.Contains(help, "-interval") ||
		!strings.Contains(help, "-database") {
		t.Errorf("run() help = %q, want usage and available flags", help)
	}
}

func TestRunRejectsPositionalArguments(t *testing.T) {
	err := run(context.Background(), []string{"unexpected"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "unexpected arguments") {
		t.Errorf("run() error = %v, want unexpected-arguments error", err)
	}
}

func TestRunReturnsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var output bytes.Buffer
	err := runIsolated(t, ctx, []string{"-interval", "24h"}, &output)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("run() error = %v, want context cancellation", err)
	}
	if output.Len() != 0 {
		t.Errorf("run() wrote %q after cancellation, want no output", output.String())
	}
}

func TestRunRestoresLatestBatteryState(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "wattfeder.db")
	firstArgs := []string{
		"-database", databasePath,
		"-interval", "12h",
		"-start", "2026-08-07T00:00:00Z",
		"-starting-battery-soc-percent", "73",
	}
	var firstOutput bytes.Buffer
	if err := run(t.Context(), firstArgs, &firstOutput); err != nil {
		t.Fatalf("first run() error = %v", err)
	}

	firstRecords := decodeRecords(t, &firstOutput)
	if len(firstRecords) != 2 {
		t.Fatalf("first run record count = %d, want 2", len(firstRecords))
	}

	secondArgs := []string{
		"-database", databasePath,
		"-interval", "24h",
		"-start", "2026-08-08T00:00:00Z",
		"-starting-battery-soc-percent", "1",
	}
	var secondOutput bytes.Buffer
	if err := run(t.Context(), secondArgs, &secondOutput); err != nil {
		t.Fatalf("second run() error = %v", err)
	}

	secondRecords := decodeRecords(t, &secondOutput)
	if len(secondRecords) != 1 {
		t.Fatalf("second run record count = %d, want 1", len(secondRecords))
	}
	wantSOC := firstRecords[len(firstRecords)-1].BatterySOCPercent
	if secondRecords[0].BatterySOCPercent != wantSOC {
		t.Errorf(
			"restored battery SOC = %v, want latest persisted value %v",
			secondRecords[0].BatterySOCPercent,
			wantSOC,
		)
	}
}

func TestRunDoesNotRedeliverDuplicateEvent(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "wattfeder.db")
	args := []string{"-database", databasePath, "-interval", "24h"}
	var firstOutput bytes.Buffer
	if err := run(t.Context(), args, &firstOutput); err != nil {
		t.Fatalf("first run() error = %v", err)
	}
	if firstOutput.Len() == 0 {
		t.Fatal("first run output is empty")
	}

	var secondOutput bytes.Buffer
	if err := run(t.Context(), args, &secondOutput); err != nil {
		t.Fatalf("duplicate run() error = %v", err)
	}
	if secondOutput.Len() != 0 {
		t.Errorf("duplicate run output = %q, want no redelivery", secondOutput.String())
	}
}

func TestRunReturnsPersistenceStartupErrorBeforeOutput(t *testing.T) {
	var output bytes.Buffer
	err := run(
		t.Context(),
		[]string{"-database", t.TempDir(), "-interval", "24h"},
		&output,
	)
	if err == nil || !strings.Contains(err.Error(), "open persistence") {
		t.Errorf("run() error = %v, want persistence startup error", err)
	}
	if output.Len() != 0 {
		t.Errorf("run() output = %q, want none after persistence startup failure", output.String())
	}
}

func runIsolated(t *testing.T, ctx context.Context, args []string, output io.Writer) error {
	t.Helper()
	databaseArgs := []string{"-database", filepath.Join(t.TempDir(), "wattfeder.db")}
	return run(ctx, append(args, databaseArgs...), output)
}

func decodeRecords(t *testing.T, input io.Reader) []application.Record {
	t.Helper()
	decoder := json.NewDecoder(input)
	var records []application.Record
	for {
		var record application.Record
		if err := decoder.Decode(&record); errors.Is(err, io.EOF) {
			return records
		} else if err != nil {
			t.Fatalf("decode record: %v", err)
		}
		records = append(records, record)
	}
}

// errorWriter makes JSON encoding fail deterministically without filesystem I/O
type errorWriter struct {
	err error
}

func (w errorWriter) Write([]byte) (int, error) {
	return 0, w.err
}
