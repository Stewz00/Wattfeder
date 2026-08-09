package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Stewz00/wattfeder/internal/household"
	"github.com/Stewz00/wattfeder/internal/persistence"
	"github.com/Stewz00/wattfeder/internal/persistence/sqlite"
)

const testDeviceID = "home-001"

var testBaseTime = time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC)

func testPolicy(t *testing.T) household.Policy {
	t.Helper()
	policy, err := household.NewPolicy(10, time.Hour)
	if err != nil {
		t.Fatalf("NewPolicy() error = %v", err)
	}
	return policy
}

func TestRunPersistentDayAcceptsFreshEventAndAppliesCommand(t *testing.T) {
	sim := &stubSimulation{steps: []simulationStep{
		{envelope: freshEnvelope("event-001", testBaseTime, 5, 1, 50, 0.30), nominalTime: testBaseTime},
	}}
	repository := &stubRepository{}

	var records []Record
	if err := RunPersistentDay(t.Context(), sim, testPolicy(t), repository, testDeviceID, func(r Record) error {
		records = append(records, r)
		return nil
	}); err != nil {
		t.Fatalf("RunPersistentDay() error = %v", err)
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

	if sim.completeCalls != 1 || sim.completedCommands[0] == nil {
		t.Errorf("completeCalls = %d, completedCommands = %v, want one non-nil command", sim.completeCalls, sim.completedCommands)
	}
	if repository.commitCalls != 1 {
		t.Fatalf("commitCalls = %d, want 1", repository.commitCalls)
	}
	committed := repository.commitResults[0]
	if committed.Telemetry == nil || committed.LatestState == nil || committed.Command == nil {
		t.Errorf("committed result = %+v, want telemetry, latest state, and command all present", committed)
	}
}

func TestRunPersistentDayDelayedEventUpdatesStateButSuppressesCommand(t *testing.T) {
	eventTime := testBaseTime
	receivedAt := testBaseTime.Add(2 * time.Hour) // delay exceeds the 1h interval
	sim := &stubSimulation{steps: []simulationStep{
		{envelope: envelopeAt("event-001", eventTime, receivedAt, 5, 1, 50, 0.30), nominalTime: receivedAt},
	}}
	repository := &stubRepository{}

	var records []Record
	if err := RunPersistentDay(t.Context(), sim, testPolicy(t), repository, testDeviceID, func(r Record) error {
		records = append(records, r)
		return nil
	}); err != nil {
		t.Fatalf("RunPersistentDay() error = %v", err)
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
	if sim.completedCommands[0] != nil {
		t.Errorf("completed command = %+v, want nil", sim.completedCommands[0])
	}
}

func TestRunPersistentDayOutOfOrderEventNeverReplacesNewerState(t *testing.T) {
	newer := freshEnvelope("event-001", testBaseTime.Add(time.Hour), 5, 1, 50, 0.30)
	older := envelopeAt("event-002", testBaseTime, testBaseTime.Add(2*time.Hour), 8, 1, 90, 0.30)
	sim := &stubSimulation{steps: []simulationStep{
		{envelope: newer, nominalTime: testBaseTime.Add(time.Hour)},
		{envelope: older, nominalTime: testBaseTime.Add(2 * time.Hour)},
	}}
	repository := &stubRepository{}

	var records []Record
	if err := RunPersistentDay(t.Context(), sim, testPolicy(t), repository, testDeviceID, func(r Record) error {
		records = append(records, r)
		return nil
	}); err != nil {
		t.Fatalf("RunPersistentDay() error = %v", err)
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
	if sim.completedCommands[1] != nil {
		t.Errorf("completed command = %+v, want nil", sim.completedCommands[1])
	}

	lastCommitted := repository.commitResults[len(repository.commitResults)-1]
	if lastCommitted.LatestState != nil {
		t.Errorf("committed LatestState = %+v, want nil", lastCommitted.LatestState)
	}
}

func TestRunPersistentDayEveryIgnoredOrRejectedObservationEmitsReason(t *testing.T) {
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
			sim := &stubSimulation{steps: []simulationStep{{envelope: tt.envelope, nominalTime: testBaseTime}}}
			repository := &stubRepository{}

			var records []Record
			if err := RunPersistentDay(t.Context(), sim, testPolicy(t), repository, testDeviceID, func(r Record) error {
				records = append(records, r)
				return nil
			}); err != nil {
				t.Fatalf("RunPersistentDay() error = %v", err)
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

func TestRunPersistentDayContinuesAfterEveryIgnoredDisposition(t *testing.T) {
	sim := &stubSimulation{steps: []simulationStep{
		{envelope: envelopeAt("event-001", testBaseTime, testBaseTime, -1, 1, 50, 0.30), nominalTime: testBaseTime},                                                         // rejected
		{envelope: nil, nominalTime: testBaseTime.Add(time.Hour)},                                                                                                           // missing
		{envelope: &household.ObservationEnvelope{SourceDeviceID: testDeviceID, ReceivedAt: testBaseTime.Add(2 * time.Hour)}, nominalTime: testBaseTime.Add(2 * time.Hour)}, // unavailable
		{envelope: freshEnvelope("event-002", testBaseTime.Add(3*time.Hour), 5, 1, 50, 0.30), nominalTime: testBaseTime.Add(3 * time.Hour)},                                 // duplicate (forced below)
		{envelope: freshEnvelope("event-003", testBaseTime.Add(4*time.Hour), 5, 1, 50, 0.30), nominalTime: testBaseTime.Add(4 * time.Hour)},                                 // valid accepted
	}}
	repository := &stubRepository{commitStatuses: []persistence.CommitStatus{
		persistence.CommitStored, persistence.CommitStored, persistence.CommitStored,
		persistence.CommitDuplicate, persistence.CommitStored,
	}}

	var records []Record
	if err := RunPersistentDay(t.Context(), sim, testPolicy(t), repository, testDeviceID, func(r Record) error {
		records = append(records, r)
		return nil
	}); err != nil {
		t.Fatalf("RunPersistentDay() error = %v", err)
	}

	if sim.nextCalls != 5 {
		t.Errorf("nextCalls = %d, want 5 (processing must continue through every ignored disposition)", sim.nextCalls)
	}
	if sim.completeCalls != 5 {
		t.Errorf("completeCalls = %d, want 5", sim.completeCalls)
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

func TestRunPersistentDayDuplicateIsCompleteNoOp(t *testing.T) {
	sim := &stubSimulation{steps: []simulationStep{
		{envelope: freshEnvelope("event-001", testBaseTime, 5, 1, 50, 0.30), nominalTime: testBaseTime},
	}}
	repository := &stubRepository{commitStatuses: []persistence.CommitStatus{persistence.CommitDuplicate}}

	var records []Record
	if err := RunPersistentDay(t.Context(), sim, testPolicy(t), repository, testDeviceID, func(r Record) error {
		records = append(records, r)
		return nil
	}); err != nil {
		t.Fatalf("RunPersistentDay() error = %v", err)
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
	if sim.completedCommands[0] != nil {
		t.Errorf("completed command = %+v, want nil", sim.completedCommands[0])
	}
}

func TestRunPersistentDayRestoresSnapshotBeforeProcessing(t *testing.T) {
	priorState := stateAt(t, "event-000", testBaseTime, 4, 1, 40, 0.28)
	sim := &stubSimulation{steps: []simulationStep{
		// same event time as the restored snapshot: must classify as history-only, not accepted
		{envelope: envelopeAt("event-001", testBaseTime, testBaseTime.Add(time.Minute), 5, 1, 50, 0.30), nominalTime: testBaseTime.Add(time.Minute)},
	}}
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
	if err := RunPersistentDay(t.Context(), sim, testPolicy(t), repository, testDeviceID, func(r Record) error {
		records = append(records, r)
		return nil
	}); err != nil {
		t.Fatalf("RunPersistentDay() error = %v", err)
	}

	if repository.snapshotCalls != 1 {
		t.Errorf("snapshotCalls = %d, want 1", repository.snapshotCalls)
	}
	if records[0].Disposition != household.DispositionHistoryOnly {
		t.Errorf("Disposition = %v, want %v (restored snapshot must seed prior state)", records[0].Disposition, household.DispositionHistoryOnly)
	}
}

func TestRunPersistentDayStopsOnSimulatorObservationError(t *testing.T) {
	wantErr := errors.New("simulator unavailable")
	sim := &stubSimulation{steps: []simulationStep{{}}, nextErr: wantErr}
	repository := &stubRepository{}

	err := RunPersistentDay(t.Context(), sim, testPolicy(t), repository, testDeviceID, func(Record) error { return nil })
	if !errors.Is(err, wantErr) {
		t.Errorf("RunPersistentDay() error = %v, want wrapped %v", err, wantErr)
	}
}

func TestRunPersistentDayStopsOnCompleteError(t *testing.T) {
	wantErr := errors.New("apply failed")
	sim := &stubSimulation{
		steps:       []simulationStep{{envelope: freshEnvelope("event-001", testBaseTime, 5, 1, 50, 0.30), nominalTime: testBaseTime}},
		completeErr: wantErr,
	}
	repository := &stubRepository{}

	err := RunPersistentDay(t.Context(), sim, testPolicy(t), repository, testDeviceID, func(Record) error { return nil })
	if !errors.Is(err, wantErr) {
		t.Errorf("RunPersistentDay() error = %v, want wrapped %v", err, wantErr)
	}
}

func TestRunPersistentDayStopsOnPersistenceCommitError(t *testing.T) {
	wantErr := errors.New("persistence unavailable")
	sim := &stubSimulation{steps: []simulationStep{
		{envelope: freshEnvelope("event-001", testBaseTime, 5, 1, 50, 0.30), nominalTime: testBaseTime},
	}}
	repository := &stubRepository{commitErr: wantErr}

	writeCalls := 0
	err := RunPersistentDay(t.Context(), sim, testPolicy(t), repository, testDeviceID, func(Record) error {
		writeCalls++
		return nil
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("RunPersistentDay() error = %v, want wrapped %v", err, wantErr)
	}
	if sim.completeCalls != 0 {
		t.Errorf("completeCalls = %d, want 0 (command must not apply after a persistence failure)", sim.completeCalls)
	}
	if writeCalls != 0 {
		t.Errorf("writeCalls = %d, want 0", writeCalls)
	}
}

func TestRunPersistentDayStopsOnWriteError(t *testing.T) {
	wantErr := errors.New("output unavailable")
	sim := &stubSimulation{steps: []simulationStep{
		{envelope: freshEnvelope("event-001", testBaseTime, 5, 1, 50, 0.30), nominalTime: testBaseTime},
	}}
	repository := &stubRepository{}

	err := RunPersistentDay(t.Context(), sim, testPolicy(t), repository, testDeviceID, func(Record) error {
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("RunPersistentDay() error = %v, want wrapped %v", err, wantErr)
	}
}

func TestRunPersistentDayStopsOnCancellation(t *testing.T) {
	sim := &stubSimulation{steps: []simulationStep{
		{envelope: freshEnvelope("event-001", testBaseTime, 5, 1, 50, 0.30), nominalTime: testBaseTime},
		{envelope: freshEnvelope("event-002", testBaseTime.Add(time.Hour), 5, 1, 50, 0.30), nominalTime: testBaseTime.Add(time.Hour)},
	}}
	repository := &stubRepository{}

	ctx, cancel := context.WithCancel(context.Background())
	writeCalls := 0
	err := RunPersistentDay(ctx, sim, testPolicy(t), repository, testDeviceID, func(Record) error {
		writeCalls++
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("RunPersistentDay() error = %v, want context cancellation", err)
	}
	if writeCalls != 1 {
		t.Errorf("writeCalls = %d, want 1 before cancellation", writeCalls)
	}
}

func TestRunPersistentDayRejectsNilRepository(t *testing.T) {
	sim := &stubSimulation{steps: []simulationStep{{envelope: freshEnvelope("event-001", testBaseTime, 5, 1, 50, 0.30), nominalTime: testBaseTime}}}

	err := RunPersistentDay(t.Context(), sim, testPolicy(t), nil, testDeviceID, func(Record) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "repository must not be nil") {
		t.Errorf("RunPersistentDay() error = %v, want nil-repository error", err)
	}
}

func TestRunPersistentDayRejectsBlankDeviceID(t *testing.T) {
	sim := &stubSimulation{steps: []simulationStep{{envelope: freshEnvelope("event-001", testBaseTime, 5, 1, 50, 0.30), nominalTime: testBaseTime}}}
	repository := &stubRepository{}

	err := RunPersistentDay(t.Context(), sim, testPolicy(t), repository, "  ", func(Record) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "device ID must not be empty") {
		t.Errorf("RunPersistentDay() error = %v, want blank-device-ID error", err)
	}
}

func TestRunPersistentDayReplayingPersistedDayReportsEveryDuplicateWithoutRedelivery(t *testing.T) {
	repository, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer repository.Close()
	if err := repository.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	newSteps := func() []simulationStep {
		return []simulationStep{
			{envelope: freshEnvelope("event-001", testBaseTime, 5, 1, 50, 0.30), nominalTime: testBaseTime},
			{envelope: freshEnvelope("event-002", testBaseTime.Add(time.Hour), 6, 1, 55, 0.30), nominalTime: testBaseTime.Add(time.Hour)},
		}
	}

	var firstRecords []Record
	firstSim := &stubSimulation{steps: newSteps()}
	if err := RunPersistentDay(t.Context(), firstSim, testPolicy(t), repository, testDeviceID, func(r Record) error {
		firstRecords = append(firstRecords, r)
		return nil
	}); err != nil {
		t.Fatalf("first RunPersistentDay() error = %v", err)
	}
	for i, record := range firstRecords {
		if record.Disposition != household.DispositionAccepted {
			t.Fatalf("first run record %d disposition = %v, want %v", i, record.Disposition, household.DispositionAccepted)
		}
	}

	var secondRecords []Record
	secondSim := &stubSimulation{steps: newSteps()}
	if err := RunPersistentDay(t.Context(), secondSim, testPolicy(t), repository, testDeviceID, func(r Record) error {
		secondRecords = append(secondRecords, r)
		return nil
	}); err != nil {
		t.Fatalf("second RunPersistentDay() error = %v", err)
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

// --- test doubles ---

type simulationStep struct {
	envelope    *household.ObservationEnvelope
	nominalTime time.Time
}

// stubSimulation lets tests force results and failures that the real simulator cannot produce on demand.
type stubSimulation struct {
	steps             []simulationStep
	nextIndex         int
	nextErr           error
	nextCalls         int
	completeErr       error
	completeCalls     int
	completedCommands []*household.Command
}

func (s *stubSimulation) IntervalsPerDay() int {
	return len(s.steps)
}

func (s *stubSimulation) NextObservation() (*household.ObservationEnvelope, time.Time, error) {
	step := s.steps[s.nextIndex]
	s.nextIndex++
	s.nextCalls++
	if s.nextErr != nil {
		return nil, time.Time{}, s.nextErr
	}
	return step.envelope, step.nominalTime, nil
}

func (s *stubSimulation) Complete(command *household.Command) error {
	s.completeCalls++
	s.completedCommands = append(s.completedCommands, command)
	return s.completeErr
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
}

func (r *stubRepository) Migrate(context.Context) error {
	return nil
}

func (r *stubRepository) Snapshot(context.Context, string) (persistence.DeviceSnapshot, bool, error) {
	r.snapshotCalls++
	return r.snapshot, r.snapshotFound, r.snapshotErr
}

func (r *stubRepository) CommitProcessing(
	_ context.Context,
	result persistence.ObservationResult,
) (persistence.CommitStatus, error) {
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
