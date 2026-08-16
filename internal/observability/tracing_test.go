package observability

import (
	"errors"
	"testing"

	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/Stewz00/wattfeder/internal/application"
	"github.com/Stewz00/wattfeder/internal/household"
)

func newTestTracer(t *testing.T) (*Tracer, *tracetest.InMemoryExporter) {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	t.Cleanup(func() { _ = provider.Shutdown(t.Context()) })
	return NewTracer(provider), exporter
}

func TestTracerRecordsOneSpanPerIntervalWithAttributes(t *testing.T) {
	tracer, exporter := newTestTracer(t)

	_, end := tracer.BeginInterval(t.Context())
	end(application.Record{
		Disposition:  household.DispositionAccepted,
		HealthStatus: household.HealthOnline,
		Decision:     household.DecisionCharge,
		EventID:      "event-001",
	}, nil)

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("span count = %d, want 1", len(spans))
	}
	span := spans[0]
	if span.Name != "interval" {
		t.Errorf("span name = %q, want %q", span.Name, "interval")
	}
	attrs := map[string]string{}
	for _, kv := range span.Attributes {
		attrs[string(kv.Key)] = kv.Value.AsString()
	}
	want := map[string]string{
		"disposition":   "accepted",
		"health_status": "online",
		"decision":      "charge",
		"event_id":      "event-001",
	}
	for key, wantValue := range want {
		if attrs[key] != wantValue {
			t.Errorf("attribute %q = %q, want %q", key, attrs[key], wantValue)
		}
	}
}

func TestTracerRecordsTheIntervalErrorAndClosesTheSpan(t *testing.T) {
	tracer, exporter := newTestTracer(t)
	wantErr := errors.New("commit failed")

	_, end := tracer.BeginInterval(t.Context())
	end(application.Record{}, wantErr)

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("span count = %d, want 1", len(spans))
	}
	span := spans[0]
	if span.Status.Code != codes.Error {
		t.Errorf("span status code = %v, want Error", span.Status.Code)
	}
	if len(span.Events) == 0 {
		t.Error("span has no recorded events, want the error recorded")
	}
	if span.EndTime.IsZero() {
		t.Error("span end time is zero, want the span closed")
	}
}
