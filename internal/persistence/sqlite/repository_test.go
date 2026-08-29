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

	for _, table := range []string{"telemetry_events", "commands", "latest_device_states", "device_health"} {
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

func TestRepositoryMigrateRejectsNonContiguousHistory(t *testing.T) {
	repository := openRepository(t, filepath.Join(t.TempDir(), "wattfeder.db"))
	defer repository.Close()

	if _, err := repository.db.Exec(createMigrationTable); err != nil {
		t.Fatalf("create migration table: %v", err)
	}
	if _, err := repository.db.Exec(
		"INSERT INTO schema_migrations (version, name, applied_at) VALUES (2, ?, ?)",
		migrations[1].name,
		time.Now().UTC().Format(timestampFormat),
	); err != nil {
		t.Fatalf("insert non-contiguous migration: %v", err)
	}

	err := repository.Migrate(t.Context())
	if err == nil || !strings.Contains(err.Error(), "contiguous") || !strings.Contains(err.Error(), "found version 2") {
		t.Fatalf("Migrate() error = %v, want non-contiguous version error", err)
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

func TestRepositoryMigrateRejectsFutureSchemaVersion(t *testing.T) {
	repository := openRepository(t, filepath.Join(t.TempDir(), "wattfeder.db"))
	defer repository.Close()

	if _, err := repository.db.Exec(createMigrationTable); err != nil {
		t.Fatalf("create migration table: %v", err)
	}
	for _, m := range migrations {
		if _, err := repository.db.Exec(
			"INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)",
			m.version,
			m.name,
			time.Now().UTC().Format(timestampFormat),
		); err != nil {
			t.Fatalf("insert migration %d: %v", m.version, err)
		}
	}
	futureVersion := len(migrations) + 1
	if _, err := repository.db.Exec(
		"INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)",
		futureVersion,
		"future migration",
		time.Now().UTC().Format(timestampFormat),
	); err != nil {
		t.Fatalf("insert future migration: %v", err)
	}

	err := repository.Migrate(t.Context())
	if err == nil || !strings.Contains(err.Error(), "newer than supported") {
		t.Fatalf("Migrate() error = %v, want newer-than-supported version error", err)
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

func TestRepositoryMigrateFromPopulatedV1BackfillsDispositionAndHealth(t *testing.T) {
	repository := openRepository(t, filepath.Join(t.TempDir(), "wattfeder.db"))
	defer repository.Close()

	if _, err := repository.db.Exec(createMigrationTable); err != nil {
		t.Fatalf("create migration table: %v", err)
	}
	if _, err := repository.db.Exec(migrations[0].sql); err != nil {
		t.Fatalf("apply v1 schema: %v", err)
	}
	if _, err := repository.db.Exec(
		"INSERT INTO schema_migrations (version, name, applied_at) VALUES (1, ?, ?)",
		migrations[0].name,
		time.Now().UTC().Format(timestampFormat),
	); err != nil {
		t.Fatalf("record v1 migration: %v", err)
	}

	receivedAt := time.Date(2026, time.August, 8, 12, 0, 1, 0, time.UTC)
	if _, err := repository.db.Exec(`
INSERT INTO telemetry_events (
    event_id, source_timestamp, received_at, device_id, pv_power_kw, load_power_kw,
    battery_soc_percent, price_eur_per_kwh
) VALUES ('event-001', ?, ?, 'home-001', 4.8, 1.9, 61, 0.28)`,
		time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC).Format(timestampFormat),
		receivedAt.Format(timestampFormat),
	); err != nil {
		t.Fatalf("seed v1 telemetry: %v", err)
	}
	if _, err := repository.db.Exec(`
INSERT INTO latest_device_states (
    device_id, last_event_id, updated_at, pv_power_kw, load_power_kw,
    battery_soc_percent, price_eur_per_kwh
) VALUES ('home-001', 'event-001', ?, 4.8, 1.9, 61, 0.28)`,
		time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC).Format(timestampFormat),
	); err != nil {
		t.Fatalf("seed v1 latest state: %v", err)
	}

	if err := repository.Migrate(t.Context()); err != nil {
		t.Fatalf("apply v2 migration: %v", err)
	}

	var disposition, reason string
	if err := repository.db.QueryRow(
		"SELECT disposition, disposition_reason FROM telemetry_events WHERE event_id = 'event-001'",
	).Scan(&disposition, &reason); err != nil {
		t.Fatalf("read backfilled disposition: %v", err)
	}
	if disposition != string(household.DispositionAccepted) {
		t.Errorf("backfilled disposition = %q, want %q", disposition, household.DispositionAccepted)
	}
	if reason != "" {
		t.Errorf("backfilled disposition reason = %q, want empty", reason)
	}

	var status, lastContactAt string
	if err := repository.db.QueryRow(
		"SELECT status, last_contact_at FROM device_health WHERE device_id = 'home-001'",
	).Scan(&status, &lastContactAt); err != nil {
		t.Fatalf("read backfilled device health: %v", err)
	}
	if status != string(household.HealthOnline) {
		t.Errorf("backfilled health status = %q, want %q", status, household.HealthOnline)
	}
	if lastContactAt != receivedAt.Format(timestampFormat) {
		t.Errorf("backfilled last contact time = %q, want %q (the prior receive time)", lastContactAt, receivedAt.Format(timestampFormat))
	}
}

func TestRepositoryCommitProcessingPersistsTraceableRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wattfeder.db")
	repository := openMigratedRepository(t, path)
	result := acceptedResult(t, "event-001", time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC))

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
JOIN device_health AS health ON health.device_id = telemetry.device_id
WHERE telemetry.event_id = ?`, string(result.Telemetry.Event.EventID)).Scan(&linkedRecords); err != nil {
		t.Fatalf("query traceable records: %v", err)
	}
	if linkedRecords != 1 {
		t.Errorf("linked record count = %d, want 1", linkedRecords)
	}

	assertLatestState(t, repository, *result.LatestState)
	if err := repository.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	repository = openMigratedRepository(t, path)
	defer repository.Close()
	assertLatestState(t, repository, *result.LatestState)
}

func TestRepositoryCommitProcessingHistoryOnlyPreservesLatestStateAndCommands(t *testing.T) {
	repository := openMigratedRepository(t, filepath.Join(t.TempDir(), "wattfeder.db"))
	defer repository.Close()

	newer := acceptedResult(t, "event-001", time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC))
	if status, err := repository.CommitProcessing(t.Context(), newer); err != nil || status != persistence.CommitStored {
		t.Fatalf("newer CommitProcessing() = (%v, %v), want (%v, nil)", status, err, persistence.CommitStored)
	}

	older := historyOnlyResult(t, "event-002", newer.Telemetry.Event.EventTime.Add(-time.Hour))
	status, err := repository.CommitProcessing(t.Context(), older)
	if err != nil {
		t.Fatalf("history-only CommitProcessing() error = %v", err)
	}
	if status != persistence.CommitStored {
		t.Errorf("history-only CommitProcessing() status = %v, want %v", status, persistence.CommitStored)
	}

	var storedCount int
	if err := repository.db.QueryRow(
		"SELECT COUNT(*) FROM telemetry_events WHERE event_id = ?",
		string(older.Telemetry.Event.EventID),
	).Scan(&storedCount); err != nil {
		t.Fatalf("count stored history-only telemetry: %v", err)
	}
	if storedCount != 1 {
		t.Errorf("stored history-only telemetry count = %d, want 1 (history-only events must still be recorded)", storedCount)
	}

	var commandCount int
	if err := repository.db.QueryRow(
		"SELECT COUNT(*) FROM commands WHERE event_id = ?", string(older.Telemetry.Event.EventID),
	).Scan(&commandCount); err != nil {
		t.Fatalf("count history-only command: %v", err)
	}
	if commandCount != 0 {
		t.Errorf("history-only command count = %d, want 0 (history-only events must not produce a command)", commandCount)
	}

	assertLatestState(t, repository, *newer.LatestState)
}

func TestRepositoryCommitProcessingDefensivelyDowngradesOlderAcceptedResult(t *testing.T) {
	repository := openMigratedRepository(t, filepath.Join(t.TempDir(), "wattfeder.db"))
	defer repository.Close()

	newer := acceptedResult(t, "event-001", time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC))
	if status, err := repository.CommitProcessing(t.Context(), newer); err != nil || status != persistence.CommitStored {
		t.Fatalf("newer CommitProcessing() = (%v, %v), want (%v, nil)", status, err, persistence.CommitStored)
	}

	// Simulate a caller that (incorrectly) classified an older event as accepted;
	// the repository must independently refuse to apply it as the latest state.
	older := acceptedResult(t, "event-002", newer.Telemetry.Event.EventTime.Add(-time.Hour))
	status, err := repository.CommitProcessing(t.Context(), older)
	if err != nil {
		t.Fatalf("older CommitProcessing() error = %v", err)
	}
	if status != persistence.CommitStored {
		t.Errorf("older CommitProcessing() status = %v, want %v", status, persistence.CommitStored)
	}

	var storedCount int
	if err := repository.db.QueryRow(
		"SELECT COUNT(*) FROM telemetry_events WHERE event_id = ?",
		string(older.Telemetry.Event.EventID),
	).Scan(&storedCount); err != nil {
		t.Fatalf("count stored older telemetry: %v", err)
	}
	if storedCount != 1 {
		t.Errorf("stored older telemetry count = %d, want 1 (older events must still be recorded)", storedCount)
	}

	var commandCount int
	if err := repository.db.QueryRow(
		"SELECT COUNT(*) FROM commands WHERE event_id = ?", string(older.Telemetry.Event.EventID),
	).Scan(&commandCount); err != nil {
		t.Fatalf("count suppressed command: %v", err)
	}
	if commandCount != 0 {
		t.Errorf("suppressed command count = %d, want 0 (a defensive downgrade must suppress command insertion)", commandCount)
	}

	assertLatestState(t, repository, *newer.LatestState)
}

func TestRepositoryCommitProcessingHealthOnlyWithoutTelemetry(t *testing.T) {
	repository := openMigratedRepository(t, filepath.Join(t.TempDir(), "wattfeder.db"))
	defer repository.Close()

	now := time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC)
	result := persistence.ObservationResult{
		DeviceID: "home-001",
		Health: household.DeviceHealth{
			Status:         household.HealthInvalid,
			Reason:         "future event time",
			TransitionTime: now,
			LastContactAt:  now,
		},
	}

	status, err := repository.CommitProcessing(t.Context(), result)
	if err != nil {
		t.Fatalf("CommitProcessing() error = %v", err)
	}
	if status != persistence.CommitStored {
		t.Errorf("CommitProcessing() status = %v, want %v", status, persistence.CommitStored)
	}

	var telemetryCount int
	if err := repository.db.QueryRow("SELECT COUNT(*) FROM telemetry_events").Scan(&telemetryCount); err != nil {
		t.Fatalf("count telemetry records: %v", err)
	}
	if telemetryCount != 0 {
		t.Errorf("telemetry record count = %d, want 0", telemetryCount)
	}

	var status2, reason string
	if err := repository.db.QueryRow(
		"SELECT status, reason FROM device_health WHERE device_id = 'home-001'",
	).Scan(&status2, &reason); err != nil {
		t.Fatalf("read device health: %v", err)
	}
	if status2 != string(household.HealthInvalid) {
		t.Errorf("health status = %q, want %q", status2, household.HealthInvalid)
	}
	if reason != "future event time" {
		t.Errorf("health reason = %q, want %q", reason, "future event time")
	}
}

func TestRepositoryCommitProcessingDuplicateChangesNothing(t *testing.T) {
	repository := openMigratedRepository(t, filepath.Join(t.TempDir(), "wattfeder.db"))
	defer repository.Close()

	original := acceptedResult(t, "event-001", time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC))
	status, err := repository.CommitProcessing(t.Context(), original)
	if err != nil || status != persistence.CommitStored {
		t.Fatalf("first CommitProcessing() = (%v, %v), want (%v, nil)", status, err, persistence.CommitStored)
	}

	duplicate := acceptedResult(t, "event-001", original.Telemetry.Event.EventTime)
	duplicate.Telemetry.ReceivedAt = duplicate.Telemetry.ReceivedAt.Add(time.Minute)
	duplicate.Command.Command.Reason = "replacement decision that must not be stored"
	duplicate.Command.CreatedAt = duplicate.Command.CreatedAt.Add(time.Minute)
	duplicate.Health.Status = household.HealthOffline
	duplicate.Health.Reason = "must not be written"
	duplicate.Health.TransitionTime = duplicate.Health.TransitionTime.Add(time.Minute)
	duplicate.Health.LastContactAt = duplicate.Health.LastContactAt.Add(time.Minute)

	status, err = repository.CommitProcessing(t.Context(), duplicate)
	if err != nil {
		t.Fatalf("duplicate CommitProcessing() error = %v", err)
	}
	if status != persistence.CommitDuplicate {
		t.Errorf("duplicate CommitProcessing() status = %v, want %v", status, persistence.CommitDuplicate)
	}
	assertLatestState(t, repository, *original.LatestState)

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

	var healthStatus string
	if err := repository.db.QueryRow(
		"SELECT status FROM device_health WHERE device_id = ?", original.DeviceID,
	).Scan(&healthStatus); err != nil {
		t.Fatalf("read duplicate-protected health: %v", err)
	}
	if healthStatus != string(original.Health.Status) {
		t.Errorf("stored health status = %q, want original %q (duplicate commits must change no durable records)", healthStatus, original.Health.Status)
	}
}

func TestRepositoryCommitProcessingRollsBackAllRecords(t *testing.T) {
	repository := openMigratedRepository(t, filepath.Join(t.TempDir(), "wattfeder.db"))
	defer repository.Close()

	first := acceptedResult(t, "event-001", time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC))
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

	second := acceptedResult(t, "event-002", first.Telemetry.Event.EventTime.Add(time.Hour))
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
	assertLatestState(t, repository, *first.LatestState)

	var healthStatus string
	if err := repository.db.QueryRow(
		"SELECT status FROM device_health WHERE device_id = ?", first.DeviceID,
	).Scan(&healthStatus); err != nil {
		t.Fatalf("read health after rollback: %v", err)
	}
	if healthStatus != string(first.Health.Status) {
		t.Errorf("health status after rollback = %q, want unchanged %q", healthStatus, first.Health.Status)
	}
}

func TestRepositoryCommitProcessingRejectsInvalidResultBeforeWriting(t *testing.T) {
	repository := openMigratedRepository(t, filepath.Join(t.TempDir(), "wattfeder.db"))
	defer repository.Close()

	result := acceptedResult(t, "event-001", time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC))
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

func TestRepositorySnapshotReportsMissingDevice(t *testing.T) {
	repository := openMigratedRepository(t, filepath.Join(t.TempDir(), "wattfeder.db"))
	defer repository.Close()

	snapshot, found, err := repository.Snapshot(t.Context(), "missing-device")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if found {
		t.Errorf("Snapshot() found = true, want false")
	}
	if snapshot != (persistence.DeviceSnapshot{}) {
		t.Errorf("Snapshot() = %+v, want zero value", snapshot)
	}
}

func TestRepositorySnapshotRestoresCompleteDeviceSnapshot(t *testing.T) {
	repository := openMigratedRepository(t, filepath.Join(t.TempDir(), "wattfeder.db"))
	defer repository.Close()

	result := acceptedResult(t, "event-001", time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC))
	if status, err := repository.CommitProcessing(t.Context(), result); err != nil || status != persistence.CommitStored {
		t.Fatalf("CommitProcessing() = (%v, %v), want (%v, nil)", status, err, persistence.CommitStored)
	}

	snapshot, found, err := repository.Snapshot(t.Context(), result.DeviceID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if !found {
		t.Fatal("Snapshot() found = false, want true")
	}
	if snapshot.State != *result.LatestState {
		t.Errorf("Snapshot() State = %+v, want %+v", snapshot.State, *result.LatestState)
	}
	if snapshot.Health != result.Health {
		t.Errorf("Snapshot() Health = %+v, want %+v", snapshot.Health, result.Health)
	}
}

func TestRepositorySnapshotReturnsHealthWithoutPriorState(t *testing.T) {
	repository := openMigratedRepository(t, filepath.Join(t.TempDir(), "wattfeder.db"))
	defer repository.Close()

	now := time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC)
	health := household.DeviceHealth{
		Status:         household.HealthInvalid,
		Reason:         "future event time",
		TransitionTime: now,
		LastContactAt:  now,
	}
	result := persistence.ObservationResult{DeviceID: "home-001", Health: health}
	if status, err := repository.CommitProcessing(t.Context(), result); err != nil || status != persistence.CommitStored {
		t.Fatalf("CommitProcessing() = (%v, %v), want (%v, nil)", status, err, persistence.CommitStored)
	}

	snapshot, found, err := repository.Snapshot(t.Context(), "home-001")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if !found {
		t.Fatal("Snapshot() found = false, want true")
	}
	if snapshot.State != (household.State{}) {
		t.Errorf("Snapshot() State = %+v, want zero value (no accepted event has ever been stored)", snapshot.State)
	}
	if snapshot.Health != health {
		t.Errorf("Snapshot() Health = %+v, want %+v", snapshot.Health, health)
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

func acceptedResult(t *testing.T, eventID household.EventID, eventTime time.Time) persistence.ObservationResult {
	t.Helper()
	event := household.Telemetry{
		EventID:           eventID,
		EventTime:         eventTime,
		DeviceID:          "home-001",
		PVPowerKW:         4.8,
		LoadPowerKW:       1.9,
		BatterySOCPercent: 61,
		PriceEURPerKWh:    0.28,
	}
	state := stateFromTelemetry(t, event)
	receivedAt := eventTime.Add(time.Second)

	return persistence.ObservationResult{
		DeviceID: event.DeviceID,
		Telemetry: &persistence.TelemetryRecord{
			Event:             event,
			ReceivedAt:        receivedAt,
			Disposition:       household.DispositionAccepted,
			DispositionReason: "",
		},
		LatestState: &state,
		Command: &persistence.CommandRecord{
			EventID: eventID,
			Command: household.Command{
				Decision: household.DecisionCharge,
				PowerKW:  2.9,
				Reason:   "PV production exceeds household load",
			},
			CreatedAt: receivedAt,
		},
		Health: household.DeviceHealth{
			Status:         household.HealthOnline,
			TransitionTime: receivedAt,
			LastContactAt:  receivedAt,
		},
	}
}

func historyOnlyResult(t *testing.T, eventID household.EventID, eventTime time.Time) persistence.ObservationResult {
	t.Helper()
	event := household.Telemetry{
		EventID:           eventID,
		EventTime:         eventTime,
		DeviceID:          "home-001",
		PVPowerKW:         4.8,
		LoadPowerKW:       1.9,
		BatterySOCPercent: 61,
		PriceEURPerKWh:    0.28,
	}
	receivedAt := eventTime.Add(time.Hour + time.Second)

	return persistence.ObservationResult{
		DeviceID: event.DeviceID,
		Telemetry: &persistence.TelemetryRecord{
			Event:             event,
			ReceivedAt:        receivedAt,
			Disposition:       household.DispositionHistoryOnly,
			DispositionReason: "event time is not strictly newer than the latest state",
		},
		Health: household.DeviceHealth{
			Status:         household.HealthOnline,
			TransitionTime: receivedAt,
			LastContactAt:  receivedAt,
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
	got, found, err := repository.Snapshot(context.Background(), want.DeviceID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if !found {
		t.Fatal("Snapshot() found = false, want true")
	}
	if got.State != want {
		t.Errorf("Snapshot() State = %+v, want %+v", got.State, want)
	}
}
