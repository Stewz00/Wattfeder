package observability

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/Stewz00/wattfeder/internal/application"
	"github.com/Stewz00/wattfeder/internal/household"
)

func newTestLogger(buf *bytes.Buffer) *Logger {
	handler := slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	return NewLogger(slog.New(handler))
}

func decodeLine(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var line map[string]any
	if err := json.NewDecoder(buf).Decode(&line); err != nil {
		t.Fatalf("decode log line: %v (buffer: %s)", err, buf.String())
	}
	return line
}

func TestLoggerEmitsIntervalFields(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	eventTime := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	receivedAt := eventTime.Add(2 * time.Second)
	record := application.Record{
		AgentID:           "agent-001",
		DeviceID:          "home-001",
		ReceivedAt:        receivedAt,
		EventID:           "event-001",
		Timestamp:         &eventTime,
		Disposition:       household.DispositionAccepted,
		DispositionReason: "",
		HealthStatus:      household.HealthOnline,
		Decision:          household.DecisionCharge,
	}

	ctx, end := logger.BeginInterval(t.Context())
	_ = ctx
	end(record, nil)

	line := decodeLine(t, &buf)
	want := map[string]string{
		"agent_id":      "agent-001",
		"device_id":     "home-001",
		"event_id":      "event-001",
		"disposition":   "accepted",
		"health_status": "online",
		"decision":      "charge",
	}
	for key, wantValue := range want {
		if got, _ := line[key].(string); got != wantValue {
			t.Errorf("field %q = %v, want %q", key, line[key], wantValue)
		}
	}
	if _, ok := line["duration_ms"]; !ok {
		t.Error("missing duration_ms field")
	}
	lag, ok := line["event_lag_seconds"].(float64)
	if !ok || lag != 2 {
		t.Errorf("event_lag_seconds = %v, want 2", line["event_lag_seconds"])
	}
}

func TestLoggerLevelsByDispositionAndError(t *testing.T) {
	tests := []struct {
		name        string
		disposition household.Disposition
		err         error
		wantLevel   string
	}{
		{name: "accepted logs info", disposition: household.DispositionAccepted, wantLevel: "INFO"},
		{name: "rejected logs warn", disposition: household.DispositionRejected, wantLevel: "WARN"},
		{name: "unavailable logs warn", disposition: household.DispositionUnavailable, wantLevel: "WARN"},
		{name: "run failure logs error", disposition: household.DispositionAccepted, err: errors.New("boom"), wantLevel: "ERROR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := newTestLogger(&buf)
			_, end := logger.BeginInterval(t.Context())
			end(application.Record{Disposition: tt.disposition}, tt.err)

			line := decodeLine(t, &buf)
			if got, _ := line["level"].(string); got != tt.wantLevel {
				t.Errorf("level = %q, want %q", got, tt.wantLevel)
			}
		})
	}
}

func TestLoggerIncludesTheTraceIDWhenTheContextCarriesASpan(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)
	tracer, _ := newTestTracer(t)

	tracedCtx, endSpan := tracer.BeginInterval(t.Context())
	_, endLog := logger.BeginInterval(tracedCtx)
	endLog(application.Record{Disposition: household.DispositionAccepted}, nil)
	endSpan(application.Record{Disposition: household.DispositionAccepted}, nil)

	line := decodeLine(t, &buf)
	traceID, ok := line["trace_id"].(string)
	if !ok || traceID == "" {
		t.Errorf("trace_id = %v, want a non-empty trace ID", line["trace_id"])
	}
}

func TestParseLevelRejectsUnknownValues(t *testing.T) {
	if _, err := ParseLevel("silly"); err == nil {
		t.Error("ParseLevel(\"silly\") error = nil, want an error")
	}
	for _, level := range []string{"debug", "info", "warn", "error"} {
		if _, err := ParseLevel(level); err != nil {
			t.Errorf("ParseLevel(%q) error = %v, want nil", level, err)
		}
	}
}
