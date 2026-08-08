# Persistence Design

## Status

The persistence records, repository contract, SQLite adapter, initial
migration, startup restore, and durable event-processing integration are
implemented.

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
database schema.

| Record | Identity and relationship | Stored values |
| --- | --- | --- |
| Telemetry | Event ID is unique. | Source timestamp, receive timestamp, device ID, PV and load power in kW, battery SOC in percent, and price in EUR/kWh. |
| Latest device state | Device ID is unique. Last event ID identifies the source telemetry. | Source timestamp and the latest telemetry measurements. |
| Command | Event ID is unique and refers to telemetry. | Creation timestamp, charge/discharge/idle decision, non-negative power magnitude in kW, and reason. |

Source, receive, and command-creation timestamps use UTC. Keeping source and
receive time separate allows later milestones to measure delivery delay without
changing event identity.

## Atomic processing boundary

`Repository.CommitProcessing` is the only operation that makes a processed
event durable. A storage adapter must perform these changes in one transaction:

1. Insert the telemetry event using event ID as the uniqueness key.
2. Insert the command linked to that event ID.
3. Replace the latest state for the device and retain the source event ID.
4. Commit all three changes.

The event is processed only after the transaction commits. If its event ID
already exists, the operation returns `CommitDuplicate` and changes nothing.
The application stops that run without applying or emitting the duplicate
command. If validation, a database write, or commit fails, the adapter returns
an error and rolls back every change. The caller can then retry the same event
ID without creating another durable command.

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
Its initial schema creates telemetry history, command history, latest device
state, and their event-ID relationships. Foreign-key and domain constraints
provide a second line of defense behind processing-result validation.

The command-line package must not contain SQL or select individual migrations.
This keeps schema changes versioned with the adapter that reads and writes the
records.

## Current assumptions and limits

- v0.2 processes one device serially, so latest-state updates do not yet resolve
  concurrent or out-of-order events.
- Duplicate identity prevents duplicate durable commands. Exactly-once delivery
  to an external battery is not yet defined.
- Event IDs are producer-owned, case-sensitive, and cannot have surrounding
  whitespace. Wattfeder validates them but does not rewrite them.
- The normal command-line application opens `wattfeder.db` by default and
  accepts another path through `-database`; scenario mode remains in-memory.
- Startup restores the latest persisted battery SOC for the configured device.
  Other latest-state measurements are replaced when the simulator emits its
  first new telemetry event.
