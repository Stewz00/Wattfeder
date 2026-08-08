# Development setup

## Required tools

- Go 1.26.5, as declared in `go.mod`
- Make for the repository commands
- Git to clone the repository

The application has no third-party Go modules. It does not require Docker, a
database, or a message broker.

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
start time, seed, device ID, battery, solar, load, and price values.

No environment variables or secrets are required. The demo uses
`scenarios/demo.json` instead of environment variables.

## Infrastructure startup

There is no external infrastructure to start. State and output stay in the
Wattfeder process.

## Application startup

Run the default simulation:

```bash
go run ./cmd/wattfeder
```

The command writes 24 newline-delimited JSON records and exits. Each record
contains telemetry and the decision for one interval.

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
- No output after interruption: A cancellation stops before the next interval.
