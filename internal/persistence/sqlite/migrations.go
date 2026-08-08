package sqlite

const createMigrationTable = `
CREATE TABLE IF NOT EXISTS schema_migrations (
	version INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	applied_at TEXT NOT NULL
);`

type migration struct {
	version int
	name    string
	sql     string
}

var migrations = []migration{
	{
		version: 1,
		name:    "initial durable processing schema",
		sql: `
CREATE TABLE telemetry_events (
	event_id TEXT PRIMARY KEY,
	source_timestamp TEXT NOT NULL,
	received_at TEXT NOT NULL,
	device_id TEXT NOT NULL,
	pv_power_kw REAL NOT NULL CHECK (pv_power_kw >= 0),
	load_power_kw REAL NOT NULL CHECK (load_power_kw >= 0),
	battery_soc_percent REAL NOT NULL CHECK (battery_soc_percent BETWEEN 0 AND 100),
	price_eur_per_kwh REAL NOT NULL CHECK (price_eur_per_kwh > 0),
	CHECK (length(trim(event_id)) > 0 AND event_id = trim(event_id)),
	CHECK (length(trim(device_id)) > 0)
);

CREATE INDEX telemetry_events_device_timestamp_idx
	ON telemetry_events (device_id, source_timestamp);

CREATE TABLE commands (
	event_id TEXT PRIMARY KEY REFERENCES telemetry_events(event_id),
	created_at TEXT NOT NULL,
	decision TEXT NOT NULL CHECK (decision IN ('charge', 'discharge', 'idle')),
	power_kw REAL NOT NULL CHECK (power_kw >= 0),
	reason TEXT NOT NULL CHECK (length(trim(reason)) > 0),
	CHECK (
		(decision IN ('charge', 'discharge') AND power_kw > 0)
		OR (decision = 'idle' AND power_kw = 0)
	)
);

CREATE TABLE latest_device_states (
	device_id TEXT PRIMARY KEY,
	last_event_id TEXT NOT NULL REFERENCES telemetry_events(event_id),
	updated_at TEXT NOT NULL,
	pv_power_kw REAL NOT NULL CHECK (pv_power_kw >= 0),
	load_power_kw REAL NOT NULL CHECK (load_power_kw >= 0),
	battery_soc_percent REAL NOT NULL CHECK (battery_soc_percent BETWEEN 0 AND 100),
	price_eur_per_kwh REAL NOT NULL CHECK (price_eur_per_kwh > 0),
	CHECK (length(trim(device_id)) > 0)
);`,
	},
}
