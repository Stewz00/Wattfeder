# Development setup

## Required tools

- Go 1.26.5, as declared in `go.mod`
- Make for the repository commands
- Git to clone the repository

The project uses a pure-Go SQLite module and does not require a separately
installed database server, Docker, or a message broker.

## Installation

```bash
git clone https://github.com/Stewz00/wattfeder.git
cd wattfeder
go version
```

No separate setup command is required.

## Configuration

The normal application accepts command-line flags. List them with:

```bash
go run ./cmd/wattfeder -help
```

The defaults run one household at one-hour intervals, waiting a real interval
between observations, until interrupted. They set the start time, seed, device
ID, battery, solar, load, price, and SQLite database path. Use `-database` to
select a different database file.

| Flag | Default | Meaning |
| --- | --- | --- |
| `-agent-id` | `agent-001` | Identity of this installed agent instance |
| `-device-id` | `home-001` | The household system this agent manages |
| `-intervals` | `0` | Number of intervals to process; `0` runs until stopped |
| `-pace` | `real` | `real` waits one interval between observations; `fast` does not wait |
| `-shutdown-grace` | `5s` | How long an in-flight commit may finish after cancellation |

`-agent-id` names the instance in the `agent_id` field of every record it
writes, but stays a runtime value: it is not written to SQLite and nothing reads
it back out of the database. `-pace fast` is what
`make run`, the demo, and the scenario runner use so they finish immediately
instead of waiting on the wall clock.

No environment variables or secrets are required. The demo uses
`scenarios/demo.json` instead of environment variables.

## Infrastructure startup

There is no external infrastructure to start. The application creates and
migrates its SQLite database when it starts.

## Application startup

Run the edge agent:

```bash
go run ./cmd/wattfeder -agent-id agent-001 -interval 5s
```

The command creates `wattfeder.db` and writes one newline-delimited JSON
record per interval, waiting a real interval between them, until interrupted
with Ctrl+C. Each record reports one interval's disposition, health, and —
when the interval produced one — its telemetry and command. Telemetry, its
command, latest device state, and device health commit atomically before the
command sink applies the command. Use `make agent` to run the same command.

To run one simulated day as fast as possible instead of waiting on the wall
clock:

```bash
go run ./cmd/wattfeder -pace fast -intervals 24
```

This writes 24 records and exits. Use `make run` to run the same command.

On a later run for the same device, the latest persisted battery SOC overrides
`-starting-battery-soc-percent`. Replaying an already committed event reports
it as a duplicate and applies no command, but does not stop the run — every
interval that follows is still processed and reported. Choose a later
`-start` value to process a new simulated day instead of replaying one.

Two agents run independently as long as each has its own `-database` (and
normally its own `-agent-id` and `-device-id`); neither one's records or
device state affect the other's.

## Test command

Run all Go tests:

```bash
make test
```

Run formatting checks, module checks, static analysis, tests, and builds:

```bash
make check
```

## Common setup failures

- `go: command not found`: Install the Go version declared in `go.mod`.
- `make: command not found`: Run the Go commands directly or install Make.
- `interval must divide 24h0m0s evenly`: Choose an interval that leaves no
  partial interval at the end of the day.
- `unexpected arguments`: Use flags before values. Run the help command to see
  accepted flags.
- `open persistence`: Verify that the `-database` parent directory exists and
  is writable.
- No output after interruption: A cancellation stops before the next interval.
- `invalid -pace value`: `-pace` accepts only `real` or `fast`.
- Every record reports `"disposition":"duplicate"` on a repeated run: The
  event IDs are already committed. Use a later `-start` value or a different
  database for an independent simulation.
