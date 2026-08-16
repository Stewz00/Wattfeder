package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Stewz00/wattfeder/internal/household"
)

// recordingObserver counts scopes opened and records the (Record, error) each one closed with,
// so tests can assert the seam's contract without a real logging or tracing adapter.
type recordingObserver struct {
	begun  int
	ended  []endedScope
	ctxKey struct{}
}

type endedScope struct {
	record Record
	err    error
}

func (o *recordingObserver) BeginInterval(ctx context.Context) (context.Context, EndInterval) {
	o.begun++
	scoped := context.WithValue(ctx, &o.ctxKey, "scoped")
	return scoped, func(record Record, err error) {
		o.ended = append(o.ended, endedScope{record: record, err: err})
	}
}

func TestRunOpensOneObserverScopePerInterval(t *testing.T) {
	source := &stubSource{envelopes: []*household.ObservationEnvelope{
		freshEnvelope("event-001", testBaseTime, 5, 1, 50, 0.30),
		freshEnvelope("event-002", testBaseTime.Add(time.Hour), 5, 1, 50, 0.30),
	}}
	observer := &recordingObserver{}

	if err := Run(t.Context(), Agent{
		Clock: NewInstantClock(testBaseTime), Source: source, Sink: &stubSink{},
		Policy: testPolicy(t), Repository: &stubRepository{}, DeviceID: testDeviceID,
		ShutdownGrace: testShutdownGrace,
		Observer:      observer,
		Write:         func(Record) error { return nil },
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// A third scope opens and closes to discover exhaustion, matching Next's own call count.
	if observer.begun != 3 {
		t.Errorf("scopes begun = %d, want 3", observer.begun)
	}
	if len(observer.ended) != 3 {
		t.Fatalf("scopes ended = %d, want 3", len(observer.ended))
	}
	for i, scope := range observer.ended[:2] {
		if scope.err != nil {
			t.Errorf("scope %d ended with err = %v, want nil", i, scope.err)
		}
		if scope.record.Disposition != household.DispositionAccepted {
			t.Errorf("scope %d record disposition = %v, want %v", i, scope.record.Disposition, household.DispositionAccepted)
		}
	}
	// Observers tell this scope apart from a real interval by exactly this pair, so both halves
	// are part of the seam's contract rather than an implementation detail.
	if observer.ended[2].err != nil {
		t.Errorf("exhaustion scope ended with err = %v, want nil (source exhaustion ends the run cleanly)", observer.ended[2].err)
	}
	if !observer.ended[2].record.IsZero() {
		t.Errorf("exhaustion scope ended with record = %+v, want the zero Record (no interval happened)", observer.ended[2].record)
	}
}

func TestRunClosesObserverScopeWithErrorOnCommitFailure(t *testing.T) {
	wantErr := errors.New("persistence unavailable")
	source := &stubSource{envelopes: []*household.ObservationEnvelope{
		freshEnvelope("event-001", testBaseTime, 5, 1, 50, 0.30),
	}}
	observer := &recordingObserver{}

	err := Run(t.Context(), Agent{
		Clock: NewInstantClock(testBaseTime), Source: source, Sink: &stubSink{},
		Policy: testPolicy(t), Repository: &stubRepository{commitErr: wantErr}, DeviceID: testDeviceID,
		ShutdownGrace: testShutdownGrace,
		Observer:      observer,
		Write:         func(Record) error { return nil },
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Run() error = %v, want wrapped %v", err, wantErr)
	}
	if len(observer.ended) != 1 {
		t.Fatalf("scopes ended = %d, want 1", len(observer.ended))
	}
	if !errors.Is(observer.ended[0].err, wantErr) {
		t.Errorf("scope ended with err = %v, want wrapped %v", observer.ended[0].err, wantErr)
	}
	if observer.ended[0].record != (Record{}) {
		t.Errorf("scope ended with record = %+v, want the zero Record (the interval failed before producing one)", observer.ended[0].record)
	}
}

func TestRunClosesObserverScopeWithErrorOnSinkFailure(t *testing.T) {
	wantErr := errors.New("apply failed")
	source := &stubSource{envelopes: []*household.ObservationEnvelope{
		freshEnvelope("event-001", testBaseTime, 5, 1, 50, 0.30),
	}}
	observer := &recordingObserver{}

	err := Run(t.Context(), Agent{
		Clock: NewInstantClock(testBaseTime), Source: source, Sink: &stubSink{err: wantErr},
		Policy: testPolicy(t), Repository: &stubRepository{}, DeviceID: testDeviceID,
		ShutdownGrace: testShutdownGrace,
		Observer:      observer,
		Write:         func(Record) error { return nil },
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Run() error = %v, want wrapped %v", err, wantErr)
	}
	if len(observer.ended) != 1 || !errors.Is(observer.ended[0].err, wantErr) {
		t.Fatalf("scope ended = %+v, want one scope ending with wrapped %v", observer.ended, wantErr)
	}
}

func TestRunPassesTheObserverScopeContextToSourceAndSink(t *testing.T) {
	source := &ctxCapturingSource{envelope: freshEnvelope("event-001", testBaseTime, 5, 1, 50, 0.30)}
	sink := &ctxCapturingSink{}
	observer := &recordingObserver{}

	if err := Run(t.Context(), Agent{
		Clock: NewInstantClock(testBaseTime), Source: source, Sink: sink,
		Policy: testPolicy(t), Repository: &stubRepository{}, DeviceID: testDeviceID,
		ShutdownGrace: testShutdownGrace,
		Observer:      observer,
		Write:         func(Record) error { return nil },
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if source.ctx.Value(&observer.ctxKey) != "scoped" {
		t.Error("Source.Next did not receive the context returned by BeginInterval")
	}
	if sink.ctx.Value(&observer.ctxKey) != "scoped" {
		t.Error("Sink.Apply did not receive a context derived from the one BeginInterval returned")
	}
}

func TestRunWithNilObserverRunsUnchanged(t *testing.T) {
	source := &stubSource{envelopes: []*household.ObservationEnvelope{
		freshEnvelope("event-001", testBaseTime, 5, 1, 50, 0.30),
	}}

	var records []Record
	if err := Run(t.Context(), Agent{
		Clock: NewInstantClock(testBaseTime), Source: source, Sink: &stubSink{},
		Policy: testPolicy(t), Repository: &stubRepository{}, DeviceID: testDeviceID,
		ShutdownGrace: testShutdownGrace,
		Write: func(r Record) error {
			records = append(records, r)
			return nil
		},
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(records) != 1 || records[0].Disposition != household.DispositionAccepted {
		t.Fatalf("records = %+v, want one accepted record", records)
	}
}

type ctxCapturingSource struct {
	envelope *household.ObservationEnvelope
	called   bool
	ctx      context.Context
}

func (s *ctxCapturingSource) Next(ctx context.Context) (*household.ObservationEnvelope, error) {
	s.ctx = ctx
	if s.called {
		return nil, ErrSourceExhausted
	}
	s.called = true
	return s.envelope, nil
}

type ctxCapturingSink struct {
	ctx context.Context
}

func (s *ctxCapturingSink) Apply(ctx context.Context, _ *household.Command) error {
	s.ctx = ctx
	return nil
}
