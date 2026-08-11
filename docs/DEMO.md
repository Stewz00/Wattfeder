# Demo

The demo runs the same durable application flow the CLI uses, against a
file-free in-memory SQLite database. Fixed scenario values enter the
simulator; each observation is classified, committed, and, if accepted and
not suppressed, produces a battery command that updates state for the next
interval. Two scenarios are included: a fault-free household day, and a day
that deliberately exercises every kind of unreliable telemetry.

## Scenarios

`scenarios/demo.json` represents one household on 1 January 2026 with no
delivery faults:

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

`scenarios/unreliable-telemetry-day.json` uses the same household shape at a
2-hour interval (12 intervals) and configures a fault schedule covering all
seven fault kinds in order: `missing_value`, `invalid_measurement`,
`missing_heartbeat`, `unavailable`, `delay`, `duplicate`, and `out_of_order`,
surrounded by ordinary deliveries. It checks the resulting decision,
disposition, and health-status sequence against expected values for every
interval — including the intervals that produce no decision at all. See
[ADR-004](engineering/adr/ADR-004-observation-disposition-and-device-health.md)
for what each fault is expected to do.

## Components started

The command starts one Go process, which opens an in-memory SQLite database
that exists only for the run and is never written to disk. That process
contains the scenario loader, household simulator, classification and
persistence code, control policy, and JSON writer. No containers, external
database, backend service, or network connection are started.

## Run the demo

From the repository root, run:

```bash
make demo
```

To run the unreliable-telemetry scenario instead:

```bash
make demo-faults
```

Go is the only runtime tool required.

In the fault run, watch the `disposition` and `health_status` fields. An
`observation_ignored` line is an interval that produced no telemetry and no
command: a rejected measurement, a missing heartbeat, or an unavailable
source. A `telemetry_produced` line with `"state_updated":false` is telemetry
that was kept but did not become the latest state. Both demos end with
`"expected_result":"matched"`.

## What happens

1. The command parses and validates the scenario file.
2. The simulator produces one observation envelope per interval, applying any
   configured fault; a missing heartbeat produces no envelope at all.
3. Each observation is classified, committed to the in-memory database, and,
   if accepted and not suppressed, produces a battery command that updates
   state for the next interval.
4. The command checks every interval's decision — and, for the
   unreliable-telemetry scenario, its disposition and health status too —
   against the scenario's expected values.

## Expected output

The command writes structured JSON lines: one `telemetry_produced` record
per interval that has telemetry to report (carrying its disposition, whether
state updated, and health), one `observation_ignored` record per interval
that does not (rejected, missing, or unavailable), and one `decision_produced`
record per interval that produced a command. `scenarios/demo.json`'s final
record is:

```json
{"event":"simulation_completed","scenario":"one-household-day","records":4,"charge_decisions":1,"discharge_decisions":3,"idle_decisions":0,"expected_result":"matched"}
```

At 06:00, discharge power is limited so the battery reaches the 20% reserve.
At 12:00, solar production exceeds load and the policy charges the battery.

## Stop and clean up

Both demos exit after their configured intervals. They create no containers
or files, and their in-memory database is discarded when the process exits.
Run the cleanup target to confirm this behavior:

```bash
make demo-clean
```

## Current limitations

- The demo runs one synthetic household for one day.
- All processing happens in one process; the in-memory database is discarded
  when the process exits, so nothing survives between separate demo runs.
- There is no backend, network transport, or device connection.
- The final battery state after the last command is not emitted.
- The physical model omits battery losses and battery power limits.
