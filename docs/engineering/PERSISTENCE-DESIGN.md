# Persistence Design

## Status

The persistence records, repository contract, SQLite adapter, two ordered
migrations, startup snapshot restore, and durable observation-processing
integration are implemented, including disposition tracking and durable
device health. See
[ADR-004](adr/ADR-004-observation-disposition-and-device-health.md) for the
disposition matrix, ordering key, and health semantics this design persists.

## Event identity

Every telemetry producer assigns an opaque event ID before Wattfeder validates
or processes the event. A retry must retain the same ID. A new source event must
receive a new ID even if its measurements equal an earlier event.

The simulator derives a stable ID by hashing the device ID and UTC event
timestamp. This is a simulation convention, not a security credential. It makes
replaying the same configured interval identifiable as a duplicate while IDs
remain distinct across devices and timestamps.

The database enforces event ID uniqueness. Device ID and timestamp are not
the database identity because later telemetry sources may legitimately assign
several events to the same device and timestamp.

## Durable records

The logical records are defined in `internal/persistence` independently of a
database schema. `ObservationResult` is the atomic outcome of processing one
interval's observation: telemetry, latest state, and command are each
optional, and are present or absent according to the observation's
disposition (see ADR-004). Device health is always required.

| Record | Identity and relationship | Stored values |
| --- | --- | --- |
| Telemetry | Event ID is unique. Present only for `accepted` and `history_only` dispositions. | Source event time, receive time, device ID, PV and load power in kW, battery SOC in percent, price in EUR/kWh, disposition, and disposition reason. |
| Latest device state | Device ID is unique. Last event ID identifies the source telemetry. Present only for `accepted` dispositions. | Source event time and the latest telemetry measurements. |
| Command | Event ID is unique and refers to telemetry. Present only when a fresh, non-delayed `accepted` event produced one. | Creation timestamp, charge/discharge/idle decision, non-negative power magnitude in kW, and reason. |
| Device health | Device ID is unique. | Status (`online`, `stale`, `offline`, `invalid`), reason, transition time, and last contact time. |

Source, receive, and command-creation timestamps use UTC. Keeping source and
receive time separate allows delivery delay to be measured and delayed
commands to be suppressed without changing event identity.

## Atomic processing boundary

`Repository.CommitProcessing` is the only operation that makes one interval's
observation result durable, and it always writes durable health, even for an
observation with no telemetry to store. A storage adapter performs these
steps in one transaction:

1. If telemetry is present, insert the event using event ID as the
   uniqueness key, carrying its disposition and disposition reason. If the
   event ID already exists, roll back and return `CommitDuplicate` — a
   duplicate changes no durable record, including health.
2. If a latest-state replacement is present, apply it only if no state
   exists yet for the device or the incoming state is strictly newer than
   the currently stored one. This defensive check is independent of the
   caller's own classification and suppresses command insertion when it
   downgrades the outcome.
3. If a command is present and the state replacement was applied, insert the
   command linked to its event ID.
4. Upsert device health.
5. Commit.

If validation, a database write, or commit fails, the adapter returns an
error and rolls back every change. The caller can then retry the same event
ID without creating another durable record. Unlike a genuine error, a
`CommitDuplicate` result is not treated as a failure: the application
continues to the next interval and reports the duplicate.

The SQLite integration must persist successfully before applying a command to
the simulator or another device. This prevents a persistence failure from
advancing battery state with an unrecorded decision. Database commit and command
delivery cannot form one transaction; recovery from a crash between those
operations remains a later delivery-semantics decision.

## Migration ownership

The concrete repository owns its schema and ordered migrations. Application
startup calls `Repository.Migrate` before restoring state or processing events.
Migration execution must be transactional where SQLite permits it, record the
applied schema version in the database, and be safe when the schema is already
current.

The SQLite adapter applies migrations in one transaction and rejects gaps,
renamed migrations, and database versions newer than the adapter understands.
Migration v1 creates telemetry history, command history, latest device
state, and their event-ID relationships. Migration v2 adds `disposition` and
`disposition_reason` columns to telemetry history, a `device_health` table,
and backfills both: existing telemetry rows default to `accepted`, and each
existing device's health is seeded from its latest telemetry's receive time.
Foreign-key and domain constraints provide a second line of defense behind
observation-result validation.

The command-line package must not contain SQL or select individual migrations.
This keeps schema changes versioned with the adapter that reads and writes the
records.

## Restoring a device snapshot

`Repository.Snapshot` restores everything a caller needs to resume processing
for a device: its latest state and its durable health. A device can have
durable health with no latest state at all — for example when every
observation seen so far was rejected, missing, or unavailable — in which case
`Snapshot` reports the health with a zero-value state.

## Current assumptions and limits

- Processing is single-writer per device, so the defensive strictly-newer
  check exists as a second line of defense rather than a concurrency-control
  mechanism; it does not yet resolve true concurrent writers.
- Duplicate identity prevents duplicate durable records. Exactly-once
  delivery to an external battery is not yet defined; a suppressed or
  delayed command is skipped for that interval, not queued or retried.
- Event IDs are producer-owned, case-sensitive, and cannot have surrounding
  whitespace. Wattfeder validates them but does not rewrite them.
- The normal command-line application opens `wattfeder.db` by default and
  accepts another path through `-database`; the demo always uses a file-free
  in-memory database.
- Startup restores the latest persisted battery SOC for the configured
  device. Other latest-state measurements are replaced when the simulator
  emits its first new accepted telemetry event.
