package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

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
