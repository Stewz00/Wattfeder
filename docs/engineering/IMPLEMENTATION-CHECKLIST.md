# Internal Implementation Checklist

This file tracks verified implementation state for development workflows. The
reader-facing progress and product scope remain in [`roadmap.md`](../roadmap.md).

## Update rule

Update this file only when the user invokes `$update-roadmap`. Check an item only
after the implementation is verified. Keep partial behavior and known gaps
visible instead of treating them as complete.

## Completed milestone: v0.1 — Single Household Simulation

### Completed foundation

- [x] Define household telemetry, latest-state, and command domain types.
- [x] Validate simulator start time, interval, device ID, capacity, and initial SOC.
- [x] Reject non-finite battery capacity and SOC values.
- [x] Give each simulator an isolated PRNG initialized from its seed.
- [x] Normalize simulation timestamps to UTC.
- [x] Generate a start-inclusive, end-exclusive 24-hour telemetry timeline.
- [x] Advance repeated simulation calls to the next 24-hour window.
- [x] Test configuration boundaries, timeline behavior, UTC normalization, and repeatability.

### Simulation models

- [x] Make the seed affect generated telemetry through reproducible variation.
- [x] Generate a plausible photovoltaic production profile.
- [x] Generate a plausible household consumption profile.
- [x] Generate a simulated electricity price profile.
- [x] Evolve battery SOC from interval energy flows.
- [x] Apply control commands to battery SOC evolution.
- [x] Enforce physical bounds and units across generated values.

### Processing and control

- [x] Validate incoming telemetry independently from simulator configuration.
- [x] Apply telemetry to household state.
- [x] Implement charge, discharge, and idle control decisions.
- [x] Configure the control policy with battery capacity and telemetry interval.
- [x] Limit discharge power to the energy available above the battery reserve.
- [x] Include a human-readable reason with every decision.
- [x] Connect simulator, state update, policy, command, and output in one data flow.

### Application behavior

- [x] Run the simulation from `go run ./cmd/wattfeder`.
- [x] Run a fixed JSON demo scenario with `make demo`.
- [x] Compare demo decisions with the expected sequence and report structured
  progress and completion records.
- [x] Emit structured, human-readable telemetry and decisions.
- [x] Handle graceful shutdown.

### Tests and documentation

- [x] Cover simulator configuration with table-driven tests.
- [x] Cover daily timeline boundaries and repeatability.
- [x] Cover profile invariants and seeded variation.
- [x] Cover telemetry validation boundaries.
- [x] Cover control-policy boundaries with table-driven tests.
- [x] Cover command validation, simulator command sequencing, application flow
  and failure paths, and cancellation.
- [x] Cover CLI help, argument validation, configuration mapping, deterministic
  output, scenario execution and flag conflicts, cancellation, and output
  failures.
- [x] Document setup, execution, assumptions, and example output in the reader guides.

## Completed milestone: v0.2 — Persistent Device State

### Completed persistence work

- [x] Assign every simulated telemetry event a stable producer-owned event ID.
- [x] Propagate event IDs through latest state, application output, and demo output.
- [x] Define telemetry, command, and latest-state records for durable processing.
- [x] Validate UTC timestamps and consistency across one durable processing result.
- [x] Define migration ownership, latest-state lookup, and one atomic repository commit operation.
- [x] Define duplicate commits as no-op results and persistence failures as all-or-nothing operations.
- [x] Document event identity, transaction boundaries, migration ownership, and current delivery limits.
- [x] Implement the SQLite schema and ordered migrations.
- [x] Implement the SQLite repository and atomic processing transaction.
- [x] Cover migration, restart, duplicate-event, rollback, and traceability behavior with integration tests.
- [x] Restore the latest state during application startup.
- [x] Connect persistence to event processing before applying simulator commands.
- [x] Cover startup restore, duplicate processing, and persistence-failure ordering through the application and CLI.

### Current behavior and gaps

The simulator emits a stable event ID, timestamp, device ID, battery SOC, and
seeded photovoltaic, household load, and electricity price profiles. It yields
one telemetry event at a time and requires one valid command before advancing
the clock and battery state. Invalid commands leave the event pending so callers
can recover. The standalone daily simulation follows uncontrolled PV-minus-load
power, while the application applies policy commands to the next interval's SOC.
Battery energy remains between empty and full and carries across repeated daily
simulations.

Valid telemetry requires a non-blank producer-owned event ID and initializes or
replaces the latest in-memory state for one device. The latest state retains the
source event ID. Invalid telemetry and events from another device are rejected
without changing that state. The deterministic policy charges from a PV surplus
while the battery is below full, and discharges a load deficit only when
electricity costs at least EUR 0.30/kWh and battery SOC is above the 20% reserve.
Discharge power is limited when necessary so the interval ends at the reserve
instead of crossing it. Other conditions produce an idle command. Every command
has a human-readable reason and a finite, non-negative power magnitude.

The CLI connects simulator, state update, policy, durable processing, command
application, and newline-delimited JSON output for one configurable 24-hour
simulation. It opens `wattfeder.db` by default or a path selected with
`-database`, applies pending migrations, and restores the latest persisted
battery SOC for the configured device before constructing the simulator. It
rejects invalid arguments and configuration, provides flag help, and treats
SIGINT or SIGTERM cancellation as a graceful shutdown.

The CLI also loads a deterministic JSON demo scenario when `-scenario` is used.
Scenario mode rejects additional configuration flags, validates the scenario
against the same fixed 24-hour model, emits separate progress records with a
shared event ID for each telemetry-decision pair, and reports whether the
produced decisions match the expected sequence. `make demo` runs the
repository's four-interval example without creating persistent state.

The persistence package defines database-independent telemetry, command, and
latest-state records. One validated processing result binds those records by
event ID and requires UTC source, receive, and command-creation timestamps. The
repository contract assigns migrations to the adapter, exposes latest-state
restore, and requires telemetry, command, and latest state to commit atomically.
A duplicate event ID changes nothing.

The SQLite adapter applies its initial schema migration transactionally and
records ordered versions. The schema stores telemetry and command history plus
one latest-state row per device, with event-ID foreign keys and domain checks.
The repository validates each processing result before atomically inserting its
telemetry and command and replacing latest state. A duplicate telemetry event
rolls back without changing any record. The application commits each result
before applying the simulator command. A failure stops before command
application and output; a duplicate stops without redelivering its command.
Latest state survives closing and reopening the database, and its battery SOC
seeds the next run for that device. Migration, restart, duplicate, rollback,
traceability, startup restore, and failure ordering are covered by automated
tests. The fixed demo remains in-memory and creates no persistent state.

### Decisions to preserve

- A simulated day is exactly 24 hours from an arbitrary configured start time.
- Timestamps are normalized to UTC.
- Event IDs are assigned by the telemetry producer before validation and remain
  stable when the same source event is retried.
- Simulated event IDs derive from device ID and UTC timestamp, so the same
  simulated interval retains its identity across time zones and replay.
- Event IDs are opaque, case-sensitive, non-blank, and cannot contain
  surrounding whitespace.
- The timeline includes its start and excludes its end.
- The interval must divide 24 hours evenly, preventing a partial final event.
- A simulator owns its clock and random stream; separate instances cannot alter
  each other's random sequence.
- Calling `SimulateDay` advances the same simulator to the next day.
- Each telemetry event reports battery SOC at its timestamp; its applied
  command determines the SOC reported at the next timestamp.
- Battery-relative power is positive when charging and negative when
  discharging. `SimulateDay` derives its passive command from PV power minus
  load power; the application uses the deterministic policy command.
- Interval energy in kWh equals power in kW multiplied by interval hours.
- Battery energy is clamped between zero and configured capacity; excess
  generation or unmet demand is implicitly exchanged with the grid.
- Photovoltaic generation is zero outside 06:00–18:00 UTC and follows a sine
  curve during daylight, scaled to 80–100% of configured peak power by the seed.
- Household load remains positive and follows smooth morning and evening peaks,
  with the evening peak higher.
- Electricity price remains positive and follows a smooth midday dip, morning
  peak, and higher evening peak.
- One seeded factor scales each profile for the whole simulated day, preserving
  its daily shape.
- Valid telemetry requires a non-blank event ID, a non-zero timestamp, and a
  non-blank device ID.
- Telemetry power measurements must be finite and non-negative, battery SOC
  must be finite and between 0% and 100%, and electricity price must be finite
  and greater than zero.
- Household state retains the latest accepted timestamp, device ID, PV power,
  load power, battery SOC, and electricity price for one device.
- The first valid telemetry event establishes the state device ID. Later events
  with a different device ID are rejected without changing state.
- Applying an invalid telemetry event leaves both initialized and uninitialized
  state unchanged.
- PV surplus produces a charge command for the surplus power unless the battery
  is full.
- A load deficit produces a discharge command for the deficit power only when
  battery SOC is above 20% and electricity price is at least EUR 0.30/kWh.
- When serving the full load deficit would cross the 20% reserve, the policy
  limits average discharge power to the energy available above the reserve for
  that interval.
- Balanced power, a full battery during surplus, the battery reserve, and a
  price below the discharge threshold produce idle commands with zero power.
- Command power is a non-negative magnitude; the decision carries its charge,
  discharge, or idle meaning.
- Charge and discharge commands require positive finite power. Idle commands
  require zero power, and every command requires a non-blank reason.
- The simulator advances its clock and battery state only after receiving one
  valid command for the pending telemetry event.
- Every control decision includes a human-readable reason derived from the
  applicable policy threshold.
- The runnable application emits one newline-delimited JSON record per interval
  for exactly one simulated day.
- Demo scenarios use the same fixed 24-hour simulation duration and provide one
  expected decision per interval.
- Scenario mode is mutually exclusive with individual CLI configuration flags.
- SIGINT and SIGTERM cancel the application without reporting an execution
  failure.
- Durable telemetry, its command, and latest device state form one atomic
  processing result linked by event ID.
- A duplicate durable event returns a duplicate status without changing any
  record. A persistence error leaves all supplied records non-durable.
- The persistence adapter owns ordered migrations; the command-line package
  does not select migrations or contain SQL.
- Normal CLI startup opens `wattfeder.db` by default, applies every pending
  migration, and then restores state for the configured device.
- A restored battery SOC overrides the configured starting SOC. Other restored
  measurements are replaced by the first new telemetry event.
- SQLite migrations run transactionally, must remain contiguous from version 1,
  and reject renamed or newer-than-supported migration history.
- SQLite enforces event-ID relationships and domain constraints in addition to
  processing-result validation.
- One latest-state row is replaced per device and retains the telemetry event ID
  that produced it.
- Persistence must commit before a command advances the simulator or another
  device. Crash recovery between database commit and command delivery remains
  undefined.
- A duplicate durable event ends the current run without applying or emitting
  its command again.

### Next task

Record the telemetry disposition matrix for duplicate, out-of-order, delayed,
missing, and invalid events, including the ordering key and the separation of
event time from receive time. Then implement that behavior, add deterministic
simulator fault cases, and introduce device health states for v0.3.

## Current milestone

- [ ] v0.3 — Unreliable Telemetry

Unstarted items:

- [ ] Separate event time from receive time in telemetry and durable records.
- [ ] Document the disposition matrix for every supported failure mode.
- [ ] Prevent delayed telemetry from replacing a newer latest state while still
  retaining it as history.
- [ ] Add deterministic fault injection to the simulator.
- [ ] Introduce online, stale, offline, and invalid device health states.
- [ ] Report rejection and ignore reasons in structured output.
- [ ] Continue processing subsequent events after a rejected or ignored event.
- [ ] Cover every supported failure mode with an automated test.

## Later milestones

- [ ] v0.4 — Single-Site Edge Runtime
- [ ] v0.5 — Observability and Local Operations
- [ ] v0.6 — Cloud Ingestion Service
- [ ] v0.7 — Offline-Capable Edge Delivery
- [ ] v0.8 — Fleet Simulation and Ingestion Load
- [ ] v0.9 — Azure Deployment with Pulumi
- [ ] v1.0 — Portfolio Readiness

The milestone titles and their exit criteria live in
[`../roadmap.md`](../roadmap.md). The goals they serve live in
[`GOALS.md`](GOALS.md).
