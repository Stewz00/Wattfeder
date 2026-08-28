// Package sqlite stores durable processing results in a SQLite database.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/Stewz00/wattfeder/internal/household"
	"github.com/Stewz00/wattfeder/internal/persistence"
	_ "modernc.org/sqlite"
)

const timestampFormat = time.RFC3339Nano

// Repository stores telemetry, commands, and latest device state in SQLite.
type Repository struct {
	db *sql.DB
}

var _ persistence.Repository = (*Repository)(nil)

// Open opens a SQLite database at path and configures connections for foreign-key enforcement.
func Open(path string) (*Repository, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("SQLite path must not be empty")
	}

	dsn, err := dataSourceName(path)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open SQLite database: %w", err)
	}

	// One connection keeps SQLite connection-local settings and transaction ordering deterministic
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("connect to SQLite database: %w", err)
	}

	return &Repository{db: db}, nil
}

func dataSourceName(path string) (string, error) {
	if path == ":memory:" {
		return "file::memory:?_busy_timeout=5000&_foreign_keys=on", nil
	}

	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve SQLite path: %w", err)
	}

	dsn := url.URL{Scheme: "file", Path: filepath.ToSlash(absolutePath)}
	query := dsn.Query()
	query.Set("_busy_timeout", "5000")
	query.Set("_foreign_keys", "on")
	dsn.RawQuery = query.Encode()
	return dsn.String(), nil
}

// Close releases the database connection.
func (r *Repository) Close() error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.Close()
}

// Migrate applies every pending schema migration in version order.
func (r *Repository) Migrate(ctx context.Context) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin schema migration: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, createMigrationTable); err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}

	applied, err := appliedMigrations(ctx, tx)
	if err != nil {
		return err
	}
	for index, migration := range migrations {
		if index < len(applied) {
			if applied[index].version != migration.version || applied[index].name != migration.name {
				return fmt.Errorf(
					"schema migration %d is %q, want %q",
					applied[index].version,
					applied[index].name,
					migration.name,
				)
			}
			continue
		}

		if _, err := tx.ExecContext(ctx, migration.sql); err != nil {
			return fmt.Errorf("apply schema migration %d (%s): %w", migration.version, migration.name, err)
		}
		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)",
			migration.version,
			migration.name,
			time.Now().UTC().Format(timestampFormat),
		); err != nil {
			return fmt.Errorf("record schema migration %d: %w", migration.version, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit schema migrations: %w", err)
	}
	return nil
}

type appliedMigration struct {
	version int
	name    string
}

func appliedMigrations(ctx context.Context, tx *sql.Tx) ([]appliedMigration, error) {
	rows, err := tx.QueryContext(ctx, "SELECT version, name FROM schema_migrations ORDER BY version")
	if err != nil {
		return nil, fmt.Errorf("read schema migrations: %w", err)
	}
	defer rows.Close()

	var applied []appliedMigration
	for rows.Next() {
		var item appliedMigration
		if err := rows.Scan(&item.version, &item.name); err != nil {
			return nil, fmt.Errorf("scan schema migration: %w", err)
		}
		if item.version != len(applied)+1 {
			return nil, fmt.Errorf("schema migrations must be contiguous from version 1; found version %d", item.version)
		}
		if item.version > len(migrations) {
			return nil, fmt.Errorf(
				"database schema version %d is newer than supported version %d",
				item.version,
				len(migrations),
			)
		}
		applied = append(applied, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read schema migrations: %w", err)
	}

	return applied, nil
}

// Snapshot returns the most recently committed device snapshot: latest state and durable
// health. found is false only when the device has never been observed.
func (r *Repository) Snapshot(ctx context.Context, deviceID string) (persistence.DeviceSnapshot, bool, error) {
	var (
		status            string
		reason            string
		transitionTime    string
		lastContactAt     string
		lastEventID       sql.NullString
		stateDeviceID     sql.NullString
		updatedAt         sql.NullString
		pvPowerKW         sql.NullFloat64
		loadPowerKW       sql.NullFloat64
		batterySOCPercent sql.NullFloat64
		priceEURPerKWh    sql.NullFloat64
	)

	err := r.db.QueryRowContext(ctx, `
SELECT
    health.status, health.reason, health.transition_time, health.last_contact_at,
    latest.last_event_id, latest.device_id, latest.updated_at, latest.pv_power_kw,
    latest.load_power_kw, latest.battery_soc_percent, latest.price_eur_per_kwh
FROM device_health AS health
LEFT JOIN latest_device_states AS latest ON latest.device_id = health.device_id
WHERE health.device_id = ?`, deviceID).Scan(
		&status, &reason, &transitionTime, &lastContactAt,
		&lastEventID, &stateDeviceID, &updatedAt, &pvPowerKW,
		&loadPowerKW, &batterySOCPercent, &priceEURPerKWh,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return persistence.DeviceSnapshot{}, false, nil
	}
	if err != nil {
		return persistence.DeviceSnapshot{}, false, fmt.Errorf("read device snapshot for device %q: %w", deviceID, err)
	}

	var snapshot persistence.DeviceSnapshot
	snapshot.Health.Status = household.DeviceHealthStatus(status)
	snapshot.Health.Reason = reason
	if snapshot.Health.TransitionTime, err = time.Parse(timestampFormat, transitionTime); err != nil {
		return persistence.DeviceSnapshot{}, false, fmt.Errorf("parse health transition time for device %q: %w", deviceID, err)
	}
	if snapshot.Health.LastContactAt, err = time.Parse(timestampFormat, lastContactAt); err != nil {
		return persistence.DeviceSnapshot{}, false, fmt.Errorf("parse health last contact time for device %q: %w", deviceID, err)
	}

	if lastEventID.Valid {
		snapshot.State.LastEventID = household.EventID(lastEventID.String)
		snapshot.State.DeviceID = stateDeviceID.String
		snapshot.State.PVPowerKW = pvPowerKW.Float64
		snapshot.State.LoadPowerKW = loadPowerKW.Float64
		snapshot.State.BatterySOCPercent = batterySOCPercent.Float64
		snapshot.State.PriceEURPerKWh = priceEURPerKWh.Float64
		if snapshot.State.UpdatedAt, err = time.Parse(timestampFormat, updatedAt.String); err != nil {
			return persistence.DeviceSnapshot{}, false, fmt.Errorf("parse latest state timestamp for device %q: %w", deviceID, err)
		}
	}

	return snapshot, true, nil
}

// CommitProcessing atomically stores one validated observation result. Telemetry, latest
// state, and command are optional; health is always written unless the event ID is a
// duplicate, in which case the commit is a complete no-op.
func (r *Repository) CommitProcessing(
	ctx context.Context,
	result persistence.ObservationResult,
) (persistence.CommitStatus, error) {
	if err := result.Validate(); err != nil {
		return 0, fmt.Errorf("validate processing result: %w", err)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin processing transaction: %w", err)
	}
	defer tx.Rollback()

	if result.Telemetry != nil {
		stored, err := insertTelemetry(ctx, tx, *result.Telemetry)
		if err != nil {
			return 0, err
		}
		if !stored {
			if err := tx.Rollback(); err != nil {
				return 0, fmt.Errorf("roll back duplicate processing transaction: %w", err)
			}
			return persistence.CommitDuplicate, nil
		}
	}

	if result.LatestState != nil {
		applied, err := replaceLatestStateIfNewer(ctx, tx, *result.LatestState)
		if err != nil {
			return 0, err
		}
		if applied && result.Command != nil {
			if err := insertCommand(ctx, tx, *result.Command); err != nil {
				return 0, err
			}
		}
	}

	if err := upsertHealth(ctx, tx, result.DeviceID, result.Health); err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit processing transaction: %w", err)
	}

	return persistence.CommitStored, nil
}

func insertTelemetry(ctx context.Context, tx *sql.Tx, record persistence.TelemetryRecord) (bool, error) {
	event := record.Event
	result, err := tx.ExecContext(ctx, `
INSERT INTO telemetry_events (
    event_id, source_timestamp, received_at, device_id, pv_power_kw, load_power_kw,
    battery_soc_percent, price_eur_per_kwh, disposition, disposition_reason
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(event_id) DO NOTHING`,
		string(event.EventID),
		event.EventTime.Format(timestampFormat),
		record.ReceivedAt.Format(timestampFormat),
		event.DeviceID,
		event.PVPowerKW,
		event.LoadPowerKW,
		event.BatterySOCPercent,
		event.PriceEURPerKWh,
		string(record.Disposition),
		record.DispositionReason,
	)
	if err != nil {
		return false, fmt.Errorf("insert telemetry event %q: %w", event.EventID, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("inspect telemetry insert for event %q: %w", event.EventID, err)
	}
	return rowsAffected == 1, nil
}

// replaceLatestStateIfNewer replaces the durable latest state only when no state exists yet
// for the device or the incoming state is strictly newer, guarding against out-of-order
// writes independently of the caller's own ordering decision.
func replaceLatestStateIfNewer(ctx context.Context, tx *sql.Tx, state household.State) (bool, error) {
	var currentUpdatedAt string
	err := tx.QueryRowContext(
		ctx, "SELECT updated_at FROM latest_device_states WHERE device_id = ?", state.DeviceID,
	).Scan(&currentUpdatedAt)

	switch {
	case errors.Is(err, sql.ErrNoRows):
		// no existing state for this device; proceed
	case err != nil:
		return false, fmt.Errorf("read current latest state for device %q: %w", state.DeviceID, err)
	default:
		current, err := time.Parse(timestampFormat, currentUpdatedAt)
		if err != nil {
			return false, fmt.Errorf("parse current latest state timestamp for device %q: %w", state.DeviceID, err)
		}
		if !state.UpdatedAt.After(current) {
			return false, nil
		}
	}

	if err := replaceLatestState(ctx, tx, state); err != nil {
		return false, err
	}
	return true, nil
}

func upsertHealth(ctx context.Context, tx *sql.Tx, deviceID string, health household.DeviceHealth) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO device_health (device_id, status, reason, transition_time, last_contact_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(device_id) DO UPDATE SET
    status = excluded.status,
    reason = excluded.reason,
    transition_time = excluded.transition_time,
    last_contact_at = excluded.last_contact_at`,
		deviceID,
		string(health.Status),
		health.Reason,
		health.TransitionTime.Format(timestampFormat),
		health.LastContactAt.Format(timestampFormat),
	)
	if err != nil {
		return fmt.Errorf("upsert device health for device %q: %w", deviceID, err)
	}
	return nil
}

func insertCommand(ctx context.Context, tx *sql.Tx, record persistence.CommandRecord) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO commands (event_id, created_at, decision, power_kw, reason)
VALUES (?, ?, ?, ?, ?)`,
		string(record.EventID),
		record.CreatedAt.Format(timestampFormat),
		string(record.Command.Decision),
		record.Command.PowerKW,
		record.Command.Reason,
	)
	if err != nil {
		return fmt.Errorf("insert command for event %q: %w", record.EventID, err)
	}
	return nil
}

func replaceLatestState(ctx context.Context, tx *sql.Tx, state household.State) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO latest_device_states (
    device_id, last_event_id, updated_at, pv_power_kw, load_power_kw,
    battery_soc_percent, price_eur_per_kwh
) VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(device_id) DO UPDATE SET
    last_event_id = excluded.last_event_id,
    updated_at = excluded.updated_at,
    pv_power_kw = excluded.pv_power_kw,
    load_power_kw = excluded.load_power_kw,
    battery_soc_percent = excluded.battery_soc_percent,
    price_eur_per_kwh = excluded.price_eur_per_kwh`,
		state.DeviceID,
		string(state.LastEventID),
		state.UpdatedAt.Format(timestampFormat),
		state.PVPowerKW,
		state.LoadPowerKW,
		state.BatterySOCPercent,
		state.PriceEURPerKWh,
	)
	if err != nil {
		return fmt.Errorf("replace latest state for device %q: %w", state.DeviceID, err)
	}
	return nil
}
