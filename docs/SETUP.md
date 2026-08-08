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

The defaults run one household for 24 hours at one-hour intervals. They set the
start time, seed, device ID, battery, solar, load, price, and SQLite database
path. Use `-database` to select a different database file.

No environment variables or secrets are required. The demo uses
`scenarios/demo.json` instead of environment variables.

## Infrastructure startup

There is no external infrastructure to start. The application creates and
migrates its SQLite database when it starts.

## Application startup

Run the default simulation:

```bash
go run ./cmd/wattfeder
```

The command creates `wattfeder.db`, writes 24 newline-delimited JSON records,
and exits. Each record contains telemetry and the decision for one interval.
Telemetry, its command, and latest device state commit atomically before the
simulator applies the command.

On a later run for the same device, the latest persisted battery SOC overrides
`-starting-battery-soc-percent`. Replaying an already committed event stops
without applying or emitting its command again. Choose a later `-start` value to
process a new simulated day.

Use `make run` to run the same command.

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
- No output from a repeated run: The first event ID is already committed. Use a
  later `-start` value or a different database for an independent simulation.
