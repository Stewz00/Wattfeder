# Internal Implementation Checklist

This file tracks verified implementation state for development workflows. The
reader-facing progress and product scope remain in [`roadmap.md`](roadmap.md).

## Update rule

Update this file only when the user invokes `$update-roadmap`. Check an item only
after the implementation is verified. Keep partial behavior and known gaps
visible instead of treating them as complete.

## Current milestone: v0.1 — Single Household Simulation

### Completed foundation

- [x] Define household telemetry, battery state, and command domain types.
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
- [ ] Apply control commands to battery SOC evolution.
- [x] Enforce physical bounds and units across generated values.

### Processing and control

- [x] Validate incoming telemetry independently from simulator configuration.
- [x] Apply telemetry to household state.
- [x] Implement charge, discharge, and idle control decisions.
- [x] Include a human-readable reason with every decision.
- [ ] Connect simulator, state update, policy, command, and output in one data flow.

### Application behavior

- [ ] Run the simulation from `go run ./cmd/wattfeder`.
- [ ] Emit structured, human-readable telemetry and decisions.
- [ ] Handle graceful shutdown.

### Tests and documentation

- [x] Cover simulator configuration with table-driven tests.
- [x] Cover daily timeline boundaries and repeatability.
- [x] Cover profile invariants and seeded variation.
- [x] Cover telemetry validation boundaries.
- [x] Cover control-policy boundaries with table-driven tests.
- [ ] Document setup, execution, assumptions, and example output in the README.

### Current behavior and gaps

The simulator emits timestamp, device ID, battery SOC, and seeded photovoltaic,
household load, and electricity price profiles. Battery SOC starts at the
configured value, then evolves from the interval's PV-minus-load energy while
remaining between empty and full. Battery state carries across repeated daily
simulation calls. Valid telemetry initializes and replaces the latest in-memory
state for one device. Invalid telemetry and events from a different device are
rejected without changing that state. The deterministic policy charges from a
PV surplus while the battery is below full, and discharges a load deficit only
when the electricity price is at least EUR 0.30/kWh and battery SOC is above
the 20% reserve. Other conditions produce an idle command. Every command has a
human-readable reason and a non-negative power magnitude. Control commands do
not affect SOC yet, and the application pipeline is not connected.

### Decisions to preserve

- A simulated day is exactly 24 hours from an arbitrary configured start time.
- Timestamps are normalized to UTC.
- The timeline includes its start and excludes its end.
- The interval must divide 24 hours evenly, preventing a partial final event.
- A simulator owns its clock and random stream; separate instances cannot alter
  each other's random sequence.
- Calling `SimulateDay` advances the same simulator to the next day.
- Each telemetry event reports battery SOC at its timestamp; that event's power
  flow determines the SOC reported at the next timestamp.
- Battery-relative power is positive when charging and negative when
  discharging. The simulator derives it as PV power minus load power.
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
- Balanced power, a full battery during surplus, the battery reserve, and a
  price below the discharge threshold produce idle commands with zero power.
- Command power is a non-negative magnitude; the decision carries its charge,
  discharge, or idle meaning.
- Every control decision includes a human-readable reason derived from the
  applicable policy threshold.

### Next task

Connect the simulator, state update, policy, command, and structured output in
the runnable application data flow.

## Later milestones

- [ ] v0.2 — Persistent Device State
- [ ] v0.3 — Unreliable Telemetry
- [ ] v0.4 — Multi-Device Processing
- [ ] v0.5 — Observability and Local Operations
- [ ] v0.6 — Pluggable Telemetry Sources
