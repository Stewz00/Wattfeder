# Internal Implementation Checklist

This file tracks verified implementation state for development workflows. The
reader-facing progress and product scope remain in [`roadmap.md`](roadmap.md).

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

### Current behavior and gaps

The simulator emits timestamp, device ID, battery SOC, and seeded photovoltaic,
household load, and electricity price profiles. It yields one telemetry event
at a time and requires one valid command before advancing the clock and battery
state. Invalid commands leave the event pending so callers can recover. The
standalone daily simulation follows uncontrolled PV-minus-load power, while the
application applies policy commands to the next interval's SOC. Battery energy
remains between empty and full and carries across repeated daily simulations.

Valid telemetry initializes and replaces the latest in-memory state for one
device. Invalid telemetry and events from another device are rejected without
changing that state. The deterministic policy charges from a PV surplus while
the battery is below full, and discharges a load deficit only when electricity
costs at least EUR 0.30/kWh and battery SOC is above the 20% reserve. Discharge
power is limited when necessary so the interval ends at the reserve instead of
crossing it. Other conditions produce an idle command. Every command has a
human-readable reason and a finite, non-negative power magnitude.

The CLI connects simulator, state update, policy, command application, and
newline-delimited JSON output for one configurable 24-hour simulation. It
rejects invalid arguments and configuration, provides flag help, and treats
SIGINT or SIGTERM cancellation as a graceful shutdown. State and output remain
process-local; persistence begins in v0.2.

The CLI also loads a deterministic JSON demo scenario when `-scenario` is used.
Scenario mode rejects additional configuration flags, validates the scenario
against the same fixed 24-hour model, emits separate progress records, and
reports whether the produced decisions match the expected sequence. `make demo`
runs the repository's four-interval example without creating persistent state.

### Decisions to preserve

- A simulated day is exactly 24 hours from an arbitrary configured start time.
- Timestamps are normalized to UTC.
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
- Valid telemetry requires a non-zero timestamp and a non-blank device ID.
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

### Next task

Begin v0.2 by defining persistent event identity, stored telemetry and command
records, migration ownership, and the atomic processing boundary before adding
SQLite-backed state.

## Later milestones

- [ ] v0.2 — Persistent Device State
- [ ] v0.3 — Unreliable Telemetry
- [ ] v0.4 — Multi-Device Processing
- [ ] v0.5 — Observability and Local Operations
- [ ] v0.6 — Pluggable Telemetry Sources
- [ ] v0.7 — Device Fleet Management
- [ ] v0.8 — Kubernetes Deployment
