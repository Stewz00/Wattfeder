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

// LatestState returns the most recently committed state for deviceID.
func (r *Repository) LatestState(ctx context.Context, deviceID string) (household.State, bool, error) {
	var state household.State
	var eventID string
	var updatedAt string
	err := r.db.QueryRowContext(ctx, `
SELECT last_event_id, device_id, updated_at, pv_power_kw, load_power_kw,
       battery_soc_percent, price_eur_per_kwh
FROM latest_device_states
WHERE device_id = ?`, deviceID).Scan(
		&eventID,
		&state.DeviceID,
		&updatedAt,
		&state.PVPowerKW,
		&state.LoadPowerKW,
		&state.BatterySOCPercent,
		&state.PriceEURPerKWh,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return household.State{}, false, nil
	}
	if err != nil {
		return household.State{}, false, fmt.Errorf("read latest state for device %q: %w", deviceID, err)
	}

	state.LastEventID = household.EventID(eventID)
	state.UpdatedAt, err = time.Parse(timestampFormat, updatedAt)
	if err != nil {
		return household.State{}, false, fmt.Errorf("parse latest state timestamp for device %q: %w", deviceID, err)
	}

	return state, true, nil
}

// CommitProcessing atomically stores one validated processing result.
func (r *Repository) CommitProcessing(
	ctx context.Context,
	result persistence.ProcessingResult,
) (persistence.CommitStatus, error) {
	if err := result.Validate(); err != nil {
		return 0, fmt.Errorf("validate processing result: %w", err)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin processing transaction: %w", err)
	}
	defer tx.Rollback()

	stored, err := insertTelemetry(ctx, tx, result.Telemetry)
	if err != nil {
		return 0, err
	}
	if !stored {
		if err := tx.Rollback(); err != nil {
			return 0, fmt.Errorf("roll back duplicate processing transaction: %w", err)
		}
		return persistence.CommitDuplicate, nil
	}

	if err := insertCommand(ctx, tx, result.Command); err != nil {
		return 0, err
	}
	if err := replaceLatestState(ctx, tx, result.LatestState); err != nil {
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
    battery_soc_percent, price_eur_per_kwh
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(event_id) DO NOTHING`,
		string(event.EventID),
		event.Timestamp.Format(timestampFormat),
		record.ReceivedAt.Format(timestampFormat),
		event.DeviceID,
		event.PVPowerKW,
		event.LoadPowerKW,
		event.BatterySOCPercent,
		event.PriceEURPerKWh,
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
