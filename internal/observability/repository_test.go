package observability

import (
	"context"
	"errors"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/Stewz00/wattfeder/internal/application"
	"github.com/Stewz00/wattfeder/internal/persistence"
)

type stubRepo struct {
	status persistence.CommitStatus
	err    error
}

func (stubRepo) Migrate(context.Context) error { return nil }
func (stubRepo) Snapshot(context.Context, string) (persistence.DeviceSnapshot, bool, error) {
	return persistence.DeviceSnapshot{}, false, nil
}
func (r stubRepo) CommitProcessing(context.Context, persistence.ObservationResult) (persistence.CommitStatus, error) {
	return r.status, r.err
}

func TestTracedRepositoryCommitSpanIsAChildOfTheIntervalSpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	tracer := NewTracer(provider)
	repo := NewTracedRepository(stubRepo{status: persistence.CommitStored}, provider)

	intervalCtx, end := tracer.BeginInterval(context.Background())
	if _, err := repo.CommitProcessing(intervalCtx, persistence.ObservationResult{DeviceID: "home-001"}); err != nil {
		t.Fatalf("CommitProcessing() error = %v", err)
	}
	end(application.Record{}, nil)

	spans := exporter.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("span count = %d, want 2 (interval + commit)", len(spans))
	}

	var interval, commit tracetest.SpanStub
	for _, span := range spans {
		switch span.Name {
		case "interval":
			interval = span
		case "commit_processing":
			commit = span
		}
	}
	if commit.Name == "" {
		t.Fatal("no commit_processing span recorded")
	}
	if commit.Parent.SpanID() != interval.SpanContext.SpanID() {
		t.Errorf("commit span parent = %v, want interval span %v", commit.Parent.SpanID(), interval.SpanContext.SpanID())
	}
}

func TestTracedRepositoryRecordsCommitFailure(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	wantErr := errors.New("commit failed")
	repo := NewTracedRepository(stubRepo{err: wantErr}, provider)

	if _, err := repo.CommitProcessing(context.Background(), persistence.ObservationResult{}); !errors.Is(err, wantErr) {
		t.Fatalf("CommitProcessing() error = %v, want %v", err, wantErr)
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("span count = %d, want 1", len(spans))
	}
	if len(spans[0].Events) == 0 {
		t.Error("commit span has no recorded events, want the error recorded")
	}
}
