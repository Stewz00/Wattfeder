# Demo

## Purpose

The demo runs one path through the current application. Fixed scenario values
enter the simulator. Telemetry then reaches the state and policy code. The
resulting command updates the battery for the next interval.

## Scenario

The file `scenarios/demo.json` represents one household on 1 January 2026. It
uses these fixed values:

| Setting | Value |
| --- | --- |
| Start | `2026-01-01T00:00:00Z` |
| Duration | 24 hours |
| Interval | 6 hours |
| Seed | 42 |
| Battery | 10 kilowatt-hours, starting at 50% |
| Solar peak | 6 kilowatts |
| Base household load | 0.4 kilowatts |
| Base electricity price | 0.30 euros per kilowatt-hour |

The expected decision sequence is discharge, discharge, charge, discharge.
The demo checks this sequence before it reports completion.

## Components started

The command starts one Go process. That process contains the scenario loader,
household simulator, telemetry state, control policy, and JSON writer. No
containers, database, backend service, or network connection are started.

## Run the demo

From the repository root, run:

```bash
make demo
```

Go is the only runtime tool required by this target.

## What happens

1. The command parses and validates `scenarios/demo.json`.
2. The simulator produces four deterministic telemetry events.
3. The application validates each event and updates the latest in-memory state.
4. The policy produces a battery command.
5. The simulator applies the command to the next interval's battery state.
6. The command checks all decisions against the expected sequence.

## Expected output

The command writes structured JSON lines. It produces four
`telemetry_produced` records and four `decision_produced` records. The final
record is:

```json
{"event":"simulation_completed","scenario":"one-household-day","records":4,"charge_decisions":1,"discharge_decisions":3,"idle_decisions":0,"expected_result":"matched"}
```

At 06:00, discharge power is limited so the battery reaches the 20% reserve.
At 12:00, solar production exceeds load and the policy charges the battery.

## Stop and clean up

The demo exits after four intervals. It creates no containers, files, or
persistent state. Run the cleanup target to confirm this behavior:

```bash
make demo-clean
```

## Current limitations

- The demo runs one synthetic household for one day.
- All processing happens in one process.
- State and output are not stored.
- There is no backend, network transport, or device connection.
- The final battery state after the last command is not emitted.
- The physical model omits battery losses and battery power limits.
