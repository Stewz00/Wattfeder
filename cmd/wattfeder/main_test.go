package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Stewz00/wattfeder/internal/application"
)

func TestRunEmitsJSONRecords(t *testing.T) {
	var output bytes.Buffer
	if err := run(context.Background(), []string{"-interval", "24h"}, &output); err != nil {
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
	if err := run(context.Background(), args, &output); err != nil {
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

	if err := run(context.Background(), args, &first); err != nil {
		t.Fatalf("first run() error = %v", err)
	}
	if err := run(context.Background(), args, &second); err != nil {
		t.Fatalf("second run() error = %v", err)
	}
	if first.String() != second.String() {
		t.Errorf("identical runs produced different output:\nfirst:  %s\nsecond: %s", first.String(), second.String())
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
	err := run(context.Background(), []string{"-interval", "24h"}, errorWriter{err: wantErr})
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
	if !strings.Contains(help, "Usage: wattfeder [options]") || !strings.Contains(help, "-interval") {
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
	err := run(ctx, []string{"-interval", "24h"}, &output)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("run() error = %v, want context cancellation", err)
	}
	if output.Len() != 0 {
		t.Errorf("run() wrote %q after cancellation, want no output", output.String())
	}
}

// errorWriter makes JSON encoding fail deterministically without filesystem I/O
type errorWriter struct {
	err error
}

func (w errorWriter) Write([]byte) (int, error) {
	return 0, w.err
}
