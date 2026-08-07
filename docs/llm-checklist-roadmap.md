# LLM Checklist Roadmap

This file tracks verified implementation state. The product scope remains in
[`roadmap.md`](roadmap.md).

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
- [ ] Generate a plausible household consumption profile.
- [ ] Generate a simulated electricity price profile.
- [ ] Evolve battery SOC from interval energy flows and control commands.
- [ ] Enforce physical bounds and units across generated values.

### Processing and control

- [ ] Validate incoming telemetry independently from simulator configuration.
- [ ] Apply telemetry to household state.
- [ ] Implement charge, discharge, and idle control decisions.
- [ ] Include a human-readable reason with every decision.
- [ ] Connect simulator, state update, policy, command, and output in one data flow.

### Application behavior

- [ ] Run the simulation from `go run ./cmd/wattfeder`.
- [ ] Emit structured, human-readable telemetry and decisions.
- [ ] Handle graceful shutdown.

### Tests and documentation

- [x] Cover simulator configuration with table-driven tests.
- [x] Cover daily timeline boundaries and repeatability.
- [ ] Cover profile invariants and seeded variation.
- [ ] Cover telemetry validation boundaries.
- [ ] Cover control-policy boundaries with table-driven tests.
- [ ] Document setup, execution, assumptions, and example output in the README.

### Current behavior and gaps

The simulator currently emits timestamp, device ID, the configured initial
battery SOC, and a seeded photovoltaic profile. PV generation follows a smooth
daylight curve with a reproducible daily weather factor and remains within its
configured peak power. Load power and electricity price remain zero, and
battery SOC does not evolve yet.

### Decisions to preserve

- A simulated day is exactly 24 hours from an arbitrary configured start time.
- Timestamps are normalized to UTC.
- The timeline includes its start and excludes its end.
- The interval must divide 24 hours evenly, preventing a partial final event.
- A simulator owns its clock and random stream; separate instances cannot alter
  each other's random sequence.
- Calling `SimulateDay` advances the same simulator to the next day.
- Photovoltaic generation is zero outside 06:00–18:00 UTC and follows a sine
  curve during daylight, scaled to 80–100% of configured peak power by the seed.

### Next task

Implement a seeded household consumption profile with explicit non-negative
power and daily shape invariants, then test repeatability and seed variation.

## Later milestones

- [ ] v0.2 — Persistent Device State
- [ ] v0.3 — Unreliable Telemetry
- [ ] v0.4 — Multi-Device Processing
- [ ] v0.5 — Observability and Local Operations
- [ ] v0.6 — Pluggable Telemetry Sources
