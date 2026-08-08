package sqlite

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Stewz00/wattfeder/internal/household"
	"github.com/Stewz00/wattfeder/internal/persistence"
)

func TestRepositoryMigrateIsOrderedAndIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wattfeder.db")
	repository := openRepository(t, path)

	for range 2 {
		if err := repository.Migrate(t.Context()); err != nil {
			t.Fatalf("Migrate() error = %v", err)
		}
	}

	var migrationCount int
	if err := repository.db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&migrationCount); err != nil {
		t.Fatalf("count schema migrations: %v", err)
	}
	if migrationCount != len(migrations) {
		t.Errorf("schema migration count = %d, want %d", migrationCount, len(migrations))
	}

	for _, table := range []string{"telemetry_events", "commands", "latest_device_states"} {
		var count int
		if err := repository.db.QueryRow(
			"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?",
			table,
		).Scan(&count); err != nil {
			t.Fatalf("find table %q: %v", table, err)
		}
		if count != 1 {
			t.Errorf("table %q count = %d, want 1", table, count)
		}
	}

	if err := repository.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	repository = openRepository(t, path)
	defer repository.Close()
	if err := repository.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate() after reopen error = %v", err)
	}
}

func TestRepositoryMigrateRejectsUnexpectedHistory(t *testing.T) {
	repository := openRepository(t, filepath.Join(t.TempDir(), "wattfeder.db"))
	defer repository.Close()

	if _, err := repository.db.Exec(createMigrationTable); err != nil {
		t.Fatalf("create migration table: %v", err)
	}
	if _, err := repository.db.Exec(
		"INSERT INTO schema_migrations (version, name, applied_at) VALUES (1, ?, ?)",
		"different migration",
		time.Now().UTC().Format(timestampFormat),
	); err != nil {
		t.Fatalf("insert unexpected migration: %v", err)
	}

	err := repository.Migrate(t.Context())
	if err == nil || !strings.Contains(err.Error(), "different migration") {
		t.Fatalf("Migrate() error = %v, want unexpected migration error", err)
	}

	var tableCount int
	if err := repository.db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'telemetry_events'",
	).Scan(&tableCount); err != nil {
		t.Fatalf("find telemetry table: %v", err)
	}
	if tableCount != 0 {
		t.Errorf("telemetry table count = %d, want 0 after migration rollback", tableCount)
	}
}

func TestRepositoryCommitProcessingPersistsTraceableRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wattfeder.db")
	repository := openMigratedRepository(t, path)
	result := processingResult(t, "event-001", time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC))

	status, err := repository.CommitProcessing(t.Context(), result)
	if err != nil {
		t.Fatalf("CommitProcessing() error = %v", err)
	}
	if status != persistence.CommitStored {
		t.Errorf("CommitProcessing() status = %v, want %v", status, persistence.CommitStored)
	}

	var linkedRecords int
	if err := repository.db.QueryRow(`
SELECT COUNT(*)
FROM telemetry_events AS telemetry
JOIN commands AS command ON command.event_id = telemetry.event_id
JOIN latest_device_states AS state ON state.last_event_id = telemetry.event_id
WHERE telemetry.event_id = ?`, string(result.Telemetry.Event.EventID)).Scan(&linkedRecords); err != nil {
		t.Fatalf("query traceable records: %v", err)
	}
	if linkedRecords != 1 {
		t.Errorf("linked record count = %d, want 1", linkedRecords)
	}

	assertLatestState(t, repository, result.LatestState)
	if err := repository.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	repository = openMigratedRepository(t, path)
	defer repository.Close()
	assertLatestState(t, repository, result.LatestState)
}

func TestRepositoryCommitProcessingDuplicateChangesNothing(t *testing.T) {
	repository := openMigratedRepository(t, filepath.Join(t.TempDir(), "wattfeder.db"))
	defer repository.Close()

	original := processingResult(t, "event-001", time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC))
	status, err := repository.CommitProcessing(t.Context(), original)
	if err != nil || status != persistence.CommitStored {
		t.Fatalf("first CommitProcessing() = (%v, %v), want (%v, nil)", status, err, persistence.CommitStored)
	}

	duplicate := original
	duplicate.Telemetry.Event.PVPowerKW = 8.4
	duplicate.Telemetry.ReceivedAt = duplicate.Telemetry.ReceivedAt.Add(time.Minute)
	duplicate.LatestState = stateFromTelemetry(t, duplicate.Telemetry.Event)
	duplicate.Command.Command.Reason = "replacement decision that must not be stored"
	duplicate.Command.CreatedAt = duplicate.Command.CreatedAt.Add(time.Minute)

	status, err = repository.CommitProcessing(t.Context(), duplicate)
	if err != nil {
		t.Fatalf("duplicate CommitProcessing() error = %v", err)
	}
	if status != persistence.CommitDuplicate {
		t.Errorf("duplicate CommitProcessing() status = %v, want %v", status, persistence.CommitDuplicate)
	}
	assertLatestState(t, repository, original.LatestState)

	var pvPower float64
	var reason string
	if err := repository.db.QueryRow(`
SELECT telemetry.pv_power_kw, command.reason
FROM telemetry_events AS telemetry
JOIN commands AS command ON command.event_id = telemetry.event_id
WHERE telemetry.event_id = ?`, string(original.Telemetry.Event.EventID)).Scan(&pvPower, &reason); err != nil {
		t.Fatalf("read duplicate-protected records: %v", err)
	}
	if pvPower != original.Telemetry.Event.PVPowerKW {
		t.Errorf("stored PV power = %v, want original %v", pvPower, original.Telemetry.Event.PVPowerKW)
	}
	if reason != original.Command.Command.Reason {
		t.Errorf("stored command reason = %q, want original %q", reason, original.Command.Command.Reason)
	}
}

func TestRepositoryCommitProcessingRollsBackAllRecords(t *testing.T) {
	repository := openMigratedRepository(t, filepath.Join(t.TempDir(), "wattfeder.db"))
	defer repository.Close()

	first := processingResult(t, "event-001", time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC))
	if status, err := repository.CommitProcessing(t.Context(), first); err != nil || status != persistence.CommitStored {
		t.Fatalf("first CommitProcessing() = (%v, %v), want (%v, nil)", status, err, persistence.CommitStored)
	}
	if _, err := repository.db.Exec(`
CREATE TRIGGER reject_latest_state_update
BEFORE UPDATE ON latest_device_states
BEGIN
    SELECT RAISE(ABORT, 'forced latest-state failure');
END;`); err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}

	second := processingResult(t, "event-002", first.Telemetry.Event.Timestamp.Add(time.Hour))
	status, err := repository.CommitProcessing(t.Context(), second)
	if err == nil || !strings.Contains(err.Error(), "forced latest-state failure") {
		t.Fatalf("second CommitProcessing() = (%v, %v), want forced failure", status, err)
	}
	if status != 0 {
		t.Errorf("failed CommitProcessing() status = %v, want zero", status)
	}

	for _, table := range []string{"telemetry_events", "commands"} {
		var count int
		if err := repository.db.QueryRow(
			"SELECT COUNT(*) FROM "+table+" WHERE event_id = ?",
			string(second.Telemetry.Event.EventID),
		).Scan(&count); err != nil {
			t.Fatalf("count rolled-back record in %s: %v", table, err)
		}
		if count != 0 {
			t.Errorf("rolled-back record count in %s = %d, want 0", table, count)
		}
	}
	assertLatestState(t, repository, first.LatestState)
}

func TestRepositoryCommitProcessingRejectsInvalidResultBeforeWriting(t *testing.T) {
	repository := openMigratedRepository(t, filepath.Join(t.TempDir(), "wattfeder.db"))
	defer repository.Close()

	result := processingResult(t, "event-001", time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC))
	result.Command.Command.Reason = ""
	status, err := repository.CommitProcessing(t.Context(), result)
	if err == nil || !strings.Contains(err.Error(), "validate processing result") {
		t.Fatalf("CommitProcessing() = (%v, %v), want validation error", status, err)
	}

	var count int
	if err := repository.db.QueryRow("SELECT COUNT(*) FROM telemetry_events").Scan(&count); err != nil {
		t.Fatalf("count telemetry records: %v", err)
	}
	if count != 0 {
		t.Errorf("telemetry record count = %d, want 0", count)
	}
}

func TestRepositoryLatestStateReportsMissingDevice(t *testing.T) {
	repository := openMigratedRepository(t, filepath.Join(t.TempDir(), "wattfeder.db"))
	defer repository.Close()

	state, found, err := repository.LatestState(t.Context(), "missing-device")
	if err != nil {
		t.Fatalf("LatestState() error = %v", err)
	}
	if found {
		t.Errorf("LatestState() found = true, want false")
	}
	if state != (household.State{}) {
		t.Errorf("LatestState() state = %+v, want zero value", state)
	}
}

func openRepository(t *testing.T, path string) *Repository {
	t.Helper()
	repository, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	return repository
}

func openMigratedRepository(t *testing.T, path string) *Repository {
	t.Helper()
	repository := openRepository(t, path)
	if err := repository.Migrate(t.Context()); err != nil {
		repository.Close()
		t.Fatalf("Migrate() error = %v", err)
	}
	return repository
}

func processingResult(t *testing.T, eventID household.EventID, timestamp time.Time) persistence.ProcessingResult {
	t.Helper()
	event := household.Telemetry{
		EventID:           eventID,
		Timestamp:         timestamp,
		DeviceID:          "home-001",
		PVPowerKW:         4.8,
		LoadPowerKW:       1.9,
		BatterySOCPercent: 61,
		PriceEURPerKWh:    0.28,
	}

	return persistence.ProcessingResult{
		Telemetry: persistence.TelemetryRecord{
			Event:      event,
			ReceivedAt: timestamp.Add(time.Second),
		},
		LatestState: stateFromTelemetry(t, event),
		Command: persistence.CommandRecord{
			EventID: eventID,
			Command: household.Command{
				Decision: household.DecisionCharge,
				PowerKW:  2.9,
				Reason:   "PV production exceeds household load",
			},
			CreatedAt: timestamp.Add(2 * time.Second),
		},
	}
}

func stateFromTelemetry(t *testing.T, event household.Telemetry) household.State {
	t.Helper()
	var state household.State
	if err := state.ApplyTelemetry(event); err != nil {
		t.Fatalf("ApplyTelemetry() error = %v", err)
	}
	return state
}

func assertLatestState(t *testing.T, repository *Repository, want household.State) {
	t.Helper()
	got, found, err := repository.LatestState(context.Background(), want.DeviceID)
	if err != nil {
		t.Fatalf("LatestState() error = %v", err)
	}
	if !found {
		t.Fatal("LatestState() found = false, want true")
	}
	if got != want {
		t.Errorf("LatestState() = %+v, want %+v", got, want)
	}
}
