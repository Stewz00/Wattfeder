package application

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Stewz00/wattfeder/internal/household"
	"github.com/Stewz00/wattfeder/internal/persistence"
	"github.com/Stewz00/wattfeder/internal/persistence/sqlite"
)

const testDeviceID = "home-001"

var testBaseTime = time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC)

const testShutdownGrace = time.Second

func testPolicy(t *testing.T) household.Policy {
	t.Helper()
	policy, err := household.NewPolicy(10, time.Hour)
	if err != nil {
		t.Fatalf("NewPolicy() error = %v", err)
	}
	return policy
}

func TestRunAcceptsFreshEventAndAppliesCommand(t *testing.T) {
	source := &stubSource{envelopes: []*household.ObservationEnvelope{
		freshEnvelope("event-001", testBaseTime, 5, 1, 50, 0.30),
	}}
	sink := &stubSink{}
	repository := &stubRepository{}

	var records []Record
	if err := Run(t.Context(), Agent{
		Clock: NewInstantClock(testBaseTime), Source: source, Sink: sink,
		Policy: testPolicy(t), Repository: repository, DeviceID: testDeviceID,
		ShutdownGrace: testShutdownGrace,
		Write: func(r Record) error {
			records = append(records, r)
			return nil
		},
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(records) != 1 {
		t.Fatalf("record count = %d, want 1", len(records))
	}
	record := records[0]
	if record.Disposition != household.DispositionAccepted {
		t.Errorf("Disposition = %v, want %v", record.Disposition, household.DispositionAccepted)
	}
	if !record.StateUpdated || !record.StoredHistory {
		t.Errorf("StateUpdated = %v, StoredHistory = %v, want both true", record.StateUpdated, record.StoredHistory)
	}
	if record.Decision == "" || record.CommandPowerKW == nil {
		t.Errorf("Decision = %q, CommandPowerKW = %v, want a command", record.Decision, record.CommandPowerKW)
	}
	if record.HealthStatus != household.HealthOnline {
		t.Errorf("HealthStatus = %v, want %v", record.HealthStatus, household.HealthOnline)
	}

	if sink.calls != 1 || sink.appliedCommands[0] == nil {
		t.Errorf("sink calls = %d, appliedCommands = %v, want one non-nil command", sink.calls, sink.appliedCommands)
	}
	if repository.commitCalls != 1 {
		t.Fatalf("commitCalls = %d, want 1", repository.commitCalls)
	}
	committed := repository.commitResults[0]
	if committed.Telemetry == nil || committed.LatestState == nil || committed.Command == nil {
		t.Errorf("committed result = %+v, want telemetry, latest state, and command all present", committed)
	}
}

func TestRunStampsEveryRecordWithTheAgentIdentity(t *testing.T) {
	source := &stubSource{envelopes: []*household.ObservationEnvelope{
		freshEnvelope("event-001", testBaseTime, 5, 1, 50, 0.30),
		nil, // an interval that produced no telemetry still names the agent that handled it
	}}

	var records []Record
	if err := Run(t.Context(), Agent{
		Clock: NewInstantClock(testBaseTime), Source: source, Sink: &stubSink{},
		Policy: testPolicy(t), Repository: &stubRepository{}, DeviceID: testDeviceID,
		ID:            "agent-042",
		ShutdownGrace: testShutdownGrace,
		Write: func(r Record) error {
			records = append(records, r)
			return nil
		},
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(records) != 2 {
		t.Fatalf("record count = %d, want 2", len(records))
	}
	for i, record := range records {
		if record.AgentID != "agent-042" {
			t.Errorf("record %d agent ID = %q, want %q", i, record.AgentID, "agent-042")
		}
		if record.DeviceID != testDeviceID {
			t.Errorf("record %d device ID = %q, want %q", i, record.DeviceID, testDeviceID)
		}
	}
}

func TestRunReportsItsFirstIntervalWithoutWaiting(t *testing.T) {
	source := &stubSource{envelopes: []*household.ObservationEnvelope{
		freshEnvelope("event-001", testBaseTime, 5, 1, 50, 0.30),
	}}

	// A real clock paced an hour apart: the agent must report its opening interval straight away
	// and only wait between the intervals that follow. The deadline is what makes this an
	// assertion rather than an hour-long hang — a loop that waits ahead of its first
	// observation reaches the deadline having written nothing, and says so in seconds.
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	writeCalls := 0
	err := Run(ctx, Agent{
		Clock: NewRealClock(), Source: source, Sink: &stubSink{},
		Policy: testPolicy(t), Repository: &stubRepository{}, DeviceID: testDeviceID,
		MaxIntervals:  1,
		ShutdownGrace: testShutdownGrace,
		Write:         func(Record) error { writeCalls++; return nil },
	})
	if err != nil {
		t.Fatalf("Run() error = %v, want nil (the first observation must not wait on the clock)", err)
	}
	if writeCalls != 1 {
		t.Errorf("writeCalls = %d, want 1", writeCalls)
	}
}

func TestRunDelayedEventUpdatesStateButSuppressesCommand(t *testing.T) {
	eventTime := testBaseTime
	receivedAt := testBaseTime.Add(2 * time.Hour) // delay exceeds the 1h interval
	source := &stubSource{envelopes: []*household.ObservationEnvelope{
		envelopeAt("event-001", eventTime, receivedAt, 5, 1, 50, 0.30),
	}}
	sink := &stubSink{}
	repository := &stubRepository{}

	var records []Record
	if err := Run(t.Context(), Agent{
		Clock: NewInstantClock(receivedAt), Source: source, Sink: sink,
		Policy: testPolicy(t), Repository: repository, DeviceID: testDeviceID,
		ShutdownGrace: testShutdownGrace,
		Write: func(r Record) error {
			records = append(records, r)
			return nil
		},
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	record := records[0]
	if record.Disposition != household.DispositionAccepted {
		t.Errorf("Disposition = %v, want %v", record.Disposition, household.DispositionAccepted)
	}
	if !record.StateUpdated {
		t.Error("StateUpdated = false, want true (a delayed event is still the newest known state)")
	}
	if record.Decision != "" || record.CommandPowerKW != nil {
		t.Errorf("Decision = %q, CommandPowerKW = %v, want no command (stale telemetry must not drive a command)", record.Decision, record.CommandPowerKW)
	}
	if record.HealthStatus != household.HealthStale {
		t.Errorf("HealthStatus = %v, want %v", record.HealthStatus, household.HealthStale)
	}
	if sink.appliedCommands[0] != nil {
		t.Errorf("applied command = %+v, want nil", sink.appliedCommands[0])
	}
}

func TestRunOutOfOrderEventNeverReplacesNewerState(t *testing.T) {
	newer := freshEnvelope("event-001", testBaseTime.Add(time.Hour), 5, 1, 50, 0.30)
	older := envelopeAt("event-002", testBaseTime, testBaseTime.Add(2*time.Hour), 8, 1, 90, 0.30)
	source := &stubSource{envelopes: []*household.ObservationEnvelope{newer, older}}
	sink := &stubSink{}
	repository := &stubRepository{}

	var records []Record
	if err := Run(t.Context(), Agent{
		Clock: NewInstantClock(testBaseTime.Add(time.Hour)), Source: source, Sink: sink,
		Policy: testPolicy(t), Repository: repository, DeviceID: testDeviceID,
		ShutdownGrace: testShutdownGrace,
		Write: func(r Record) error {
			records = append(records, r)
			return nil
		},
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	second := records[1]
	if second.Disposition != household.DispositionHistoryOnly {
		t.Errorf("Disposition = %v, want %v", second.Disposition, household.DispositionHistoryOnly)
	}
	if second.StateUpdated {
		t.Error("StateUpdated = true, want false (an out-of-order event must never replace newer state)")
	}
	if second.DispositionReason == "" {
		t.Error("DispositionReason is empty, want a structured reason")
	}
	if sink.appliedCommands[1] != nil {
		t.Errorf("applied command = %+v, want nil", sink.appliedCommands[1])
	}

	lastCommitted := repository.commitResults[len(repository.commitResults)-1]
	if lastCommitted.LatestState != nil {
		t.Errorf("committed LatestState = %+v, want nil", lastCommitted.LatestState)
	}
}

func TestRunEveryIgnoredOrRejectedObservationEmitsReason(t *testing.T) {
	tests := []struct {
		name            string
		envelope        *household.ObservationEnvelope
		wantDisposition household.Disposition
	}{
		{
			name:            "missing value",
			envelope:        envelopeMissingValue("event-001", testBaseTime),
			wantDisposition: household.DispositionRejected,
		},
		{
			name:            "invalid measurement",
			envelope:        envelopeAt("event-001", testBaseTime, testBaseTime, -1, 1, 50, 0.30),
			wantDisposition: household.DispositionRejected,
		},
		{
			name:            "missing heartbeat",
			envelope:        nil,
			wantDisposition: household.DispositionMissing,
		},
		{
			name:            "unavailable",
			envelope:        &household.ObservationEnvelope{SourceDeviceID: testDeviceID, ReceivedAt: testBaseTime},
			wantDisposition: household.DispositionUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := &stubSource{envelopes: []*household.ObservationEnvelope{tt.envelope}}
			sink := &stubSink{}
			repository := &stubRepository{}

			var records []Record
			if err := Run(t.Context(), Agent{
				Clock: NewInstantClock(testBaseTime), Source: source, Sink: sink,
				Policy: testPolicy(t), Repository: repository, DeviceID: testDeviceID,
				ShutdownGrace: testShutdownGrace,
				Write: func(r Record) error {
					records = append(records, r)
					return nil
				},
			}); err != nil {
				t.Fatalf("Run() error = %v", err)
			}

			record := records[0]
			if record.Disposition != tt.wantDisposition {
				t.Errorf("Disposition = %v, want %v", record.Disposition, tt.wantDisposition)
			}
			if record.DispositionReason == "" {
				t.Error("DispositionReason is empty, want a structured reason")
			}
			if record.Decision != "" {
				t.Errorf("Decision = %q, want empty", record.Decision)
			}
			if record.Timestamp != nil {
				t.Errorf("Timestamp = %v, want nil (no telemetry stored)", record.Timestamp)
			}
		})
	}
}

func TestRunContinuesAfterEveryIgnoredDisposition(t *testing.T) {
	source := &stubSource{envelopes: []*household.ObservationEnvelope{
		envelopeAt("event-001", testBaseTime, testBaseTime, -1, 1, 50, 0.30), // rejected
		nil, // missing
		{SourceDeviceID: testDeviceID, ReceivedAt: testBaseTime.Add(2 * time.Hour)}, // unavailable
		freshEnvelope("event-002", testBaseTime.Add(3*time.Hour), 5, 1, 50, 0.30),   // duplicate (forced below)
		freshEnvelope("event-003", testBaseTime.Add(4*time.Hour), 5, 1, 50, 0.30),   // valid accepted
	}}
	sink := &stubSink{}
	repository := &stubRepository{commitStatuses: []persistence.CommitStatus{
		persistence.CommitStored, persistence.CommitStored, persistence.CommitStored,
		persistence.CommitDuplicate, persistence.CommitStored,
	}}

	var records []Record
	if err := Run(t.Context(), Agent{
		Clock: NewInstantClock(testBaseTime), Source: source, Sink: sink,
		Policy: testPolicy(t), Repository: repository, DeviceID: testDeviceID,
		ShutdownGrace: testShutdownGrace,
		Write: func(r Record) error {
			records = append(records, r)
			return nil
		},
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if source.nextCalls != 6 {
		t.Errorf("nextCalls = %d, want 6 (processing must continue through every ignored disposition; the 6th call discovers exhaustion)", source.nextCalls)
	}
	if sink.calls != 5 {
		t.Errorf("sink calls = %d, want 5", sink.calls)
	}
	if len(records) != 5 {
		t.Fatalf("record count = %d, want 5 (one record per interval)", len(records))
	}

	wantDispositions := []household.Disposition{
		household.DispositionRejected,
		household.DispositionMissing,
		household.DispositionUnavailable,
		household.DispositionDuplicate,
		household.DispositionAccepted,
	}
	for i, want := range wantDispositions {
		if records[i].Disposition != want {
			t.Errorf("record %d disposition = %v, want %v", i, records[i].Disposition, want)
		}
	}

	last := records[4]
	if last.Decision == "" {
		t.Error("final record decision is empty, want a command for the later valid event")
	}
}

func TestRunDuplicateIsApplyNoOp(t *testing.T) {
	source := &stubSource{envelopes: []*household.ObservationEnvelope{
		freshEnvelope("event-001", testBaseTime, 5, 1, 50, 0.30),
	}}
	sink := &stubSink{}
	repository := &stubRepository{commitStatuses: []persistence.CommitStatus{persistence.CommitDuplicate}}

	var records []Record
	if err := Run(t.Context(), Agent{
		Clock: NewInstantClock(testBaseTime), Source: source, Sink: sink,
		Policy: testPolicy(t), Repository: repository, DeviceID: testDeviceID,
		ShutdownGrace: testShutdownGrace,
		Write: func(r Record) error {
			records = append(records, r)
			return nil
		},
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	record := records[0]
	if record.Disposition != household.DispositionDuplicate {
		t.Errorf("Disposition = %v, want %v", record.Disposition, household.DispositionDuplicate)
	}
	if record.StateUpdated || record.StoredHistory {
		t.Errorf("StateUpdated = %v, StoredHistory = %v, want both false", record.StateUpdated, record.StoredHistory)
	}
	if record.Decision != "" {
		t.Errorf("Decision = %q, want empty", record.Decision)
	}
	if sink.appliedCommands[0] != nil {
		t.Errorf("applied command = %+v, want nil", sink.appliedCommands[0])
	}
}

func TestRunRestoresSnapshotBeforeProcessing(t *testing.T) {
	priorState := stateAt(t, "event-000", testBaseTime, 4, 1, 40, 0.28)
	source := &stubSource{envelopes: []*household.ObservationEnvelope{
		// same event time as the restored snapshot: must classify as history-only, not accepted
		envelopeAt("event-001", testBaseTime, testBaseTime.Add(time.Minute), 5, 1, 50, 0.30),
	}}
	sink := &stubSink{}
	repository := &stubRepository{
		snapshotFound: true,
		snapshot: persistence.DeviceSnapshot{
			State: priorState,
			Health: household.DeviceHealth{
				Status: household.HealthOnline, TransitionTime: testBaseTime, LastContactAt: testBaseTime,
			},
		},
	}

	var records []Record
	if err := Run(t.Context(), Agent{
		Clock: NewInstantClock(testBaseTime.Add(time.Minute)), Source: source, Sink: sink,
		Policy: testPolicy(t), Repository: repository, DeviceID: testDeviceID,
		ShutdownGrace: testShutdownGrace,
		Write: func(r Record) error {
			records = append(records, r)
			return nil
		},
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if repository.snapshotCalls != 1 {
		t.Errorf("snapshotCalls = %d, want 1", repository.snapshotCalls)
	}
	if records[0].Disposition != household.DispositionHistoryOnly {
		t.Errorf("Disposition = %v, want %v (restored snapshot must seed prior state)", records[0].Disposition, household.DispositionHistoryOnly)
	}
}

func TestRunStopsOnSourceObservationError(t *testing.T) {
	wantErr := errors.New("source unavailable")
	source := &stubSource{nextErr: wantErr}
	sink := &stubSink{}
	repository := &stubRepository{}

	err := Run(t.Context(), Agent{
		Clock: NewInstantClock(testBaseTime), Source: source, Sink: sink,
		Policy: testPolicy(t), Repository: repository, DeviceID: testDeviceID,
		ShutdownGrace: testShutdownGrace,
		Write:         func(Record) error { return nil },
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("Run() error = %v, want wrapped %v", err, wantErr)
	}
}

func TestRunEndsCleanlyWhenSourceIsExhausted(t *testing.T) {
	source := &stubSource{envelopes: []*household.ObservationEnvelope{
		freshEnvelope("event-001", testBaseTime, 5, 1, 50, 0.30),
	}}
	sink := &stubSink{}
	repository := &stubRepository{}

	writeCalls := 0
	err := Run(t.Context(), Agent{
		Clock: NewInstantClock(testBaseTime), Source: source, Sink: sink,
		Policy: testPolicy(t), Repository: repository, DeviceID: testDeviceID,
		ShutdownGrace: testShutdownGrace,
		Write: func(Record) error {
			writeCalls++
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v, want nil (ErrSourceExhausted ends the run cleanly)", err)
	}
	if writeCalls != 1 {
		t.Errorf("writeCalls = %d, want 1", writeCalls)
	}
	if source.nextCalls != 2 {
		t.Errorf("nextCalls = %d, want 2 (the second call discovers exhaustion)", source.nextCalls)
	}
}

func TestRunStopsOnSinkApplyError(t *testing.T) {
	wantErr := errors.New("apply failed")
	source := &stubSource{envelopes: []*household.ObservationEnvelope{
		freshEnvelope("event-001", testBaseTime, 5, 1, 50, 0.30),
	}}
	sink := &stubSink{err: wantErr}
	repository := &stubRepository{}

	err := Run(t.Context(), Agent{
		Clock: NewInstantClock(testBaseTime), Source: source, Sink: sink,
		Policy: testPolicy(t), Repository: repository, DeviceID: testDeviceID,
		ShutdownGrace: testShutdownGrace,
		Write:         func(Record) error { return nil },
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("Run() error = %v, want wrapped %v", err, wantErr)
	}
}

func TestRunStopsOnPersistenceCommitError(t *testing.T) {
	wantErr := errors.New("persistence unavailable")
	source := &stubSource{envelopes: []*household.ObservationEnvelope{
		freshEnvelope("event-001", testBaseTime, 5, 1, 50, 0.30),
	}}
	sink := &stubSink{}
	repository := &stubRepository{commitErr: wantErr}

	writeCalls := 0
	err := Run(t.Context(), Agent{
		Clock: NewInstantClock(testBaseTime), Source: source, Sink: sink,
		Policy: testPolicy(t), Repository: repository, DeviceID: testDeviceID,
		ShutdownGrace: testShutdownGrace,
		Write: func(Record) error {
			writeCalls++
			return nil
		},
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("Run() error = %v, want wrapped %v", err, wantErr)
	}
	if sink.calls != 0 {
		t.Errorf("sink calls = %d, want 0 (command must not apply after a persistence failure)", sink.calls)
	}
	if writeCalls != 0 {
		t.Errorf("writeCalls = %d, want 0", writeCalls)
	}
}

func TestRunStopsOnWriteError(t *testing.T) {
	wantErr := errors.New("output unavailable")
	source := &stubSource{envelopes: []*household.ObservationEnvelope{
		freshEnvelope("event-001", testBaseTime, 5, 1, 50, 0.30),
	}}
	sink := &stubSink{}
	repository := &stubRepository{}

	err := Run(t.Context(), Agent{
		Clock: NewInstantClock(testBaseTime), Source: source, Sink: sink,
		Policy: testPolicy(t), Repository: repository, DeviceID: testDeviceID,
		ShutdownGrace: testShutdownGrace,
		Write: func(Record) error {
			return wantErr
		},
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("Run() error = %v, want wrapped %v", err, wantErr)
	}
}

func TestRunStopsOnCancellationBetweenIntervals(t *testing.T) {
	source := &stubSource{envelopes: []*household.ObservationEnvelope{
		freshEnvelope("event-001", testBaseTime, 5, 1, 50, 0.30),
		freshEnvelope("event-002", testBaseTime.Add(time.Hour), 5, 1, 50, 0.30),
	}}
	sink := &stubSink{}
	repository := &stubRepository{}

	ctx, cancel := context.WithCancel(context.Background())
	writeCalls := 0
	err := Run(ctx, Agent{
		Clock: NewInstantClock(testBaseTime), Source: source, Sink: sink,
		Policy: testPolicy(t), Repository: repository, DeviceID: testDeviceID,
		ShutdownGrace: testShutdownGrace,
		Write: func(Record) error {
			writeCalls++
			cancel()
			return nil
		},
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want context cancellation", err)
	}
	if writeCalls != 1 {
		t.Errorf("writeCalls = %d, want 1 before cancellation", writeCalls)
	}
}

func TestRunStopsOnCancellationBeforeFirstInterval(t *testing.T) {
	source := &stubSource{envelopes: []*household.ObservationEnvelope{
		freshEnvelope("event-001", testBaseTime, 5, 1, 50, 0.30),
	}}
	sink := &stubSink{}
	repository := &stubRepository{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := Run(ctx, Agent{
		Clock: NewInstantClock(testBaseTime), Source: source, Sink: sink,
		Policy: testPolicy(t), Repository: repository, DeviceID: testDeviceID,
		ShutdownGrace: testShutdownGrace,
		Write:         func(Record) error { return nil },
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want context cancellation", err)
	}
	if source.nextCalls != 0 {
		t.Errorf("nextCalls = %d, want 0 (cancellation must be checked before Next is called)", source.nextCalls)
	}
	if repository.commitCalls != 0 {
		t.Errorf("commitCalls = %d, want 0", repository.commitCalls)
	}
	if sink.calls != 0 {
		t.Errorf("sink calls = %d, want 0", sink.calls)
	}
}

func TestRunProcessesPastTwentyFourIntervalsWhenUnbounded(t *testing.T) {
	source := &infiniteSource{start: testBaseTime}
	sink := &stubSink{}
	repository := &stubRepository{}

	const wantRecords = 30
	ctx, cancel := context.WithCancel(context.Background())
	writeCalls := 0
	err := Run(ctx, Agent{
		Clock: NewInstantClock(testBaseTime), Source: source, Sink: sink,
		Policy: testPolicy(t), Repository: repository, DeviceID: testDeviceID,
		ShutdownGrace: testShutdownGrace,
		Write: func(Record) error {
			writeCalls++
			if writeCalls == wantRecords {
				cancel()
			}
			return nil
		},
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want context cancellation", err)
	}
	if writeCalls != wantRecords {
		t.Errorf("writeCalls = %d, want %d (the loop must not stop at a fixed day boundary)", writeCalls, wantRecords)
	}
}

func TestRunStopsAfterMaxIntervalsReached(t *testing.T) {
	source := &infiniteSource{start: testBaseTime}
	sink := &stubSink{}
	repository := &stubRepository{}

	const maxIntervals = 3
	writeCalls := 0
	err := Run(t.Context(), Agent{
		Clock: NewInstantClock(testBaseTime), Source: source, Sink: sink,
		Policy: testPolicy(t), Repository: repository, DeviceID: testDeviceID,
		MaxIntervals:  maxIntervals,
		ShutdownGrace: testShutdownGrace,
		Write: func(Record) error {
			writeCalls++
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if writeCalls != maxIntervals {
		t.Errorf("writeCalls = %d, want %d", writeCalls, maxIntervals)
	}
	if source.nextCalls != maxIntervals {
		t.Errorf("nextCalls = %d, want %d (Next must not be called a 4th time)", source.nextCalls, maxIntervals)
	}
}

func TestRunShutdownGraceExceededReturnsAnError(t *testing.T) {
	source := &stubSource{envelopes: []*household.ObservationEnvelope{
		freshEnvelope("event-001", testBaseTime, 5, 1, 50, 0.30),
	}}
	sink := &stubSink{}
	repository := &stubRepository{blockUntilCommitContextDone: true}

	const tinyGrace = 5 * time.Millisecond
	err := Run(t.Context(), Agent{
		Clock: NewInstantClock(testBaseTime), Source: source, Sink: sink,
		Policy: testPolicy(t), Repository: repository, DeviceID: testDeviceID,
		ShutdownGrace: tinyGrace,
		Write:         func(Record) error { return nil },
	})
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Run() error = %v, want a deadline-exceeded error", err)
	}
}

func TestRunGoroutineCountIsStableOverManyIntervals(t *testing.T) {
	source := &infiniteSource{start: testBaseTime}
	sink := &stubSink{}
	repository := &stubRepository{}

	// The real clock is the only Clock that starts a goroutine per tick, so it is the one this
	// test has to run against; the instant clock starts none and would pass while a leak grew
	// unchecked. A microsecond interval keeps 1000 real waits down to a millisecond in total,
	// which is why the loop can be paced by the wall clock here without any test sleeping.
	policy, err := household.NewPolicy(10, time.Microsecond)
	if err != nil {
		t.Fatalf("NewPolicy() error = %v", err)
	}

	before := runtime.NumGoroutine()
	const intervals = 1000
	if err := Run(t.Context(), Agent{
		Clock: NewRealClock(), Source: source, Sink: sink,
		Policy: policy, Repository: repository, DeviceID: testDeviceID,
		MaxIntervals:  intervals,
		ShutdownGrace: testShutdownGrace,
		Write:         func(Record) error { return nil },
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Each tick's goroutine exits once it has fired, but "returned" and "no longer counted" are
	// not the same instant, so yield before reading rather than trusting the first sample. A
	// real leak grows by one per interval and stays far above the tolerance regardless.
	const tolerance = 5
	after := runtime.NumGoroutine()
	for range 100 {
		if after <= before+tolerance {
			break
		}
		runtime.Gosched()
		after = runtime.NumGoroutine()
	}
	if after > before+tolerance {
		t.Errorf("goroutine count after %d intervals = %d, want at most %d (before = %d)", intervals, after, before+tolerance, before)
	}
}

func TestRunRejectsNilRepository(t *testing.T) {
	source := &stubSource{envelopes: []*household.ObservationEnvelope{freshEnvelope("event-001", testBaseTime, 5, 1, 50, 0.30)}}
	sink := &stubSink{}

	err := Run(t.Context(), Agent{
		Clock: NewInstantClock(testBaseTime), Source: source, Sink: sink,
		Policy: testPolicy(t), Repository: nil, DeviceID: testDeviceID,
		ShutdownGrace: testShutdownGrace,
		Write:         func(Record) error { return nil },
	})
	if err == nil || !strings.Contains(err.Error(), "repository must not be nil") {
		t.Errorf("Run() error = %v, want nil-repository error", err)
	}
}

func TestRunRejectsBlankDeviceID(t *testing.T) {
	source := &stubSource{envelopes: []*household.ObservationEnvelope{freshEnvelope("event-001", testBaseTime, 5, 1, 50, 0.30)}}
	sink := &stubSink{}
	repository := &stubRepository{}

	err := Run(t.Context(), Agent{
		Clock: NewInstantClock(testBaseTime), Source: source, Sink: sink,
		Policy: testPolicy(t), Repository: repository, DeviceID: "  ",
		ShutdownGrace: testShutdownGrace,
		Write:         func(Record) error { return nil },
	})
	if err == nil || !strings.Contains(err.Error(), "device ID must not be empty") {
		t.Errorf("Run() error = %v, want blank-device-ID error", err)
	}
}

func TestRunReplayingPersistedIntervalsReportsEveryDuplicateWithoutRedelivery(t *testing.T) {
	repository, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer repository.Close()
	if err := repository.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	newEnvelopes := func() []*household.ObservationEnvelope {
		return []*household.ObservationEnvelope{
			freshEnvelope("event-001", testBaseTime, 5, 1, 50, 0.30),
			freshEnvelope("event-002", testBaseTime.Add(time.Hour), 6, 1, 55, 0.30),
		}
	}

	var firstRecords []Record
	firstSource := &stubSource{envelopes: newEnvelopes()}
	if err := Run(t.Context(), Agent{
		Clock: NewInstantClock(testBaseTime), Source: firstSource, Sink: &stubSink{},
		Policy: testPolicy(t), Repository: repository, DeviceID: testDeviceID,
		ShutdownGrace: testShutdownGrace,
		Write: func(r Record) error {
			firstRecords = append(firstRecords, r)
			return nil
		},
	}); err != nil {
		t.Fatalf("first Run() error = %v", err)
	}
	for i, record := range firstRecords {
		if record.Disposition != household.DispositionAccepted {
			t.Fatalf("first run record %d disposition = %v, want %v", i, record.Disposition, household.DispositionAccepted)
		}
	}

	var secondRecords []Record
	secondSource := &stubSource{envelopes: newEnvelopes()}
	if err := Run(t.Context(), Agent{
		Clock: NewInstantClock(testBaseTime), Source: secondSource, Sink: &stubSink{},
		Policy: testPolicy(t), Repository: repository, DeviceID: testDeviceID,
		ShutdownGrace: testShutdownGrace,
		Write: func(r Record) error {
			secondRecords = append(secondRecords, r)
			return nil
		},
	}); err != nil {
		t.Fatalf("second Run() error = %v", err)
	}

	if len(secondRecords) != len(firstRecords) {
		t.Fatalf("second run record count = %d, want %d (every duplicate must still be reported)", len(secondRecords), len(firstRecords))
	}
	for i, record := range secondRecords {
		if record.Disposition != household.DispositionDuplicate {
			t.Errorf("second run record %d disposition = %v, want %v", i, record.Disposition, household.DispositionDuplicate)
		}
		if record.StateUpdated {
			t.Errorf("second run record %d state_updated = true, want false", i)
		}
		if record.Decision != "" {
			t.Errorf("second run record %d decision = %q, want empty (must not redeliver)", i, record.Decision)
		}
	}
}

func TestRunCommitsAnObservationClassifiedBeforeCancellation(t *testing.T) {
	repository, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer repository.Close()
	if err := repository.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	source := &cancelOnNextSource{
		envelope: freshEnvelope("event-001", testBaseTime, 5, 1, 50, 0.30),
		cancel:   cancel,
	}
	sink := &stubSink{}

	writeCalls := 0
	err = Run(ctx, Agent{
		Clock: NewInstantClock(testBaseTime), Source: source, Sink: sink,
		Policy: testPolicy(t), Repository: repository, DeviceID: testDeviceID,
		ShutdownGrace: testShutdownGrace,
		Write: func(r Record) error {
			writeCalls++
			if r.Disposition != household.DispositionAccepted {
				t.Errorf("record disposition = %v, want %v", r.Disposition, household.DispositionAccepted)
			}
			return nil
		},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context cancellation", err)
	}
	if writeCalls != 1 {
		t.Fatalf("writeCalls = %d, want 1 (the observation in flight when cancellation arrived must still be reported)", writeCalls)
	}

	snapshot, found, err := repository.Snapshot(context.Background(), testDeviceID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if !found {
		t.Fatal("Snapshot() found = false, want the classified observation to be durable despite cancellation")
	}
	if snapshot.State.BatterySOCPercent != 50 {
		t.Errorf("restored battery SOC = %v, want 50", snapshot.State.BatterySOCPercent)
	}
}

// --- test doubles ---

// cancelOnNextSource returns one envelope and cancels ctx as it does, so a test can prove that
// an observation already handed to Run survives cancellation that arrives before it commits.
type cancelOnNextSource struct {
	envelope *household.ObservationEnvelope
	cancel   context.CancelFunc
	called   bool
}

func (s *cancelOnNextSource) Next(context.Context) (*household.ObservationEnvelope, error) {
	if s.called {
		return nil, ErrSourceExhausted
	}
	s.called = true
	s.cancel()
	return s.envelope, nil
}

// stubSource replays a fixed sequence of envelopes, then reports ErrSourceExhausted; it lets
// tests force results and failures a real telemetry adapter cannot produce on demand.
type stubSource struct {
	envelopes []*household.ObservationEnvelope
	nextIndex int
	nextErr   error
	nextCalls int
}

func (s *stubSource) Next(context.Context) (*household.ObservationEnvelope, error) {
	s.nextCalls++
	if s.nextErr != nil {
		return nil, s.nextErr
	}
	if s.nextIndex >= len(s.envelopes) {
		return nil, ErrSourceExhausted
	}
	envelope := s.envelopes[s.nextIndex]
	s.nextIndex++
	return envelope, nil
}

// infiniteSource never exhausts, for tests proving the loop is not bounded to a fixed count.
type infiniteSource struct {
	start     time.Time
	step      int
	nextCalls int
}

func (s *infiniteSource) Next(context.Context) (*household.ObservationEnvelope, error) {
	s.nextCalls++
	eventTime := s.start.Add(time.Duration(s.step) * time.Hour)
	s.step++
	return freshEnvelope(household.EventID(eventTime.Format(time.RFC3339)), eventTime, 5, 1, 50, 0.30), nil
}

type stubSink struct {
	err             error
	calls           int
	appliedCommands []*household.Command
}

func (s *stubSink) Apply(_ context.Context, command *household.Command) error {
	s.calls++
	s.appliedCommands = append(s.appliedCommands, command)
	return s.err
}

type stubRepository struct {
	snapshot      persistence.DeviceSnapshot
	snapshotFound bool
	snapshotErr   error
	snapshotCalls int

	// commitStatuses, when non-empty, returns one status per call in order, holding the last entry for any further calls
	commitStatuses []persistence.CommitStatus
	commitErr      error
	commitCalls    int
	commitResults  []persistence.ObservationResult

	// blockUntilCommitContextDone makes CommitProcessing wait for the commit context to end
	// instead of returning immediately, so tests can prove a shutdown grace timeout fires.
	blockUntilCommitContextDone bool
}

func (r *stubRepository) Migrate(context.Context) error {
	return nil
}

func (r *stubRepository) Snapshot(context.Context, string) (persistence.DeviceSnapshot, bool, error) {
	r.snapshotCalls++
	return r.snapshot, r.snapshotFound, r.snapshotErr
}

func (r *stubRepository) CommitProcessing(
	ctx context.Context,
	result persistence.ObservationResult,
) (persistence.CommitStatus, error) {
	if r.blockUntilCommitContextDone {
		<-ctx.Done()
		return 0, ctx.Err()
	}

	status := persistence.CommitStored
	if len(r.commitStatuses) > 0 {
		index := r.commitCalls
		if index >= len(r.commitStatuses) {
			index = len(r.commitStatuses) - 1
		}
		status = r.commitStatuses[index]
	}

	r.commitCalls++
	r.commitResults = append(r.commitResults, result)
	return status, r.commitErr
}

func freshEnvelope(eventID household.EventID, eventTime time.Time, pv, load, soc, price float64) *household.ObservationEnvelope {
	return envelopeAt(eventID, eventTime, eventTime, pv, load, soc, price)
}

func envelopeAt(
	eventID household.EventID, eventTime, receivedAt time.Time, pv, load, soc, price float64,
) *household.ObservationEnvelope {
	return &household.ObservationEnvelope{
		SourceDeviceID: testDeviceID,
		ReceivedAt:     receivedAt,
		Telemetry: &household.RawTelemetry{
			EventID:           eventID,
			EventTime:         eventTime,
			DeviceID:          testDeviceID,
			PVPowerKW:         &pv,
			LoadPowerKW:       &load,
			BatterySOCPercent: &soc,
			PriceEURPerKWh:    &price,
		},
		Available: true,
	}
}

func envelopeMissingValue(eventID household.EventID, eventTime time.Time) *household.ObservationEnvelope {
	load, soc, price := 1.0, 50.0, 0.30
	return &household.ObservationEnvelope{
		SourceDeviceID: testDeviceID,
		ReceivedAt:     eventTime,
		Telemetry: &household.RawTelemetry{
			EventID:           eventID,
			EventTime:         eventTime,
			DeviceID:          testDeviceID,
			PVPowerKW:         nil,
			LoadPowerKW:       &load,
			BatterySOCPercent: &soc,
			PriceEURPerKWh:    &price,
		},
		Available: true,
	}
}

func stateAt(t *testing.T, eventID household.EventID, eventTime time.Time, pv, load, soc, price float64) household.State {
	t.Helper()
	var state household.State
	if err := state.ApplyTelemetry(household.Telemetry{
		EventID: eventID, EventTime: eventTime, DeviceID: testDeviceID,
		PVPowerKW: pv, LoadPowerKW: load, BatterySOCPercent: soc, PriceEURPerKWh: price,
	}); err != nil {
		t.Fatalf("ApplyTelemetry() error = %v", err)
	}
	return state
}
