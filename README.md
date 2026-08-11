# Wattfeder

Wattfeder is a Go edge agent for a household energy system. It reads telemetry
from a household — solar production, load, battery charge, electricity price —
and decides every interval whether to charge, discharge, or idle the battery.

Telemetry in the real world is not clean. It arrives twice, arrives late,
arrives out of order, arrives broken, or does not arrive at all. Wattfeder is
built around that fact. Every observation gets one explicit outcome, and one
bad observation never stops the ones after it.

Current version: **v0.4**. One household, one long-running edge process, local
SQLite storage. There is no cloud service yet.

## Try it in one command

```bash
make demo         # A clean day: four intervals, four decisions
make demo-faults  # A day with duplicate, late, broken, and missing telemetry
```

Both need Go 1.26.5 and nothing else. No Docker, no database server, no network.
They print newline-delimited JSON and check the result against the expected
sequence. See [Demo](docs/DEMO.md) for what each line means.

```bash
make agent        # The edge agent, real pacing, until Ctrl+C
make check        # Format, analyze, test, and build
```

## How it works

```text
Simulator → observation → classify → store → command → back to the battery
```

1. The **simulator** produces one observation per interval. It can inject
   faults on purpose, from a scenario file.
2. **`household.Classify`** gives that observation exactly one disposition:
   `accepted`, `history_only`, `duplicate`, `rejected`, `missing`, or
   `unavailable`. It also gives the device one health status: `online`,
   `stale`, `offline`, or `invalid`.
3. **SQLite** stores the telemetry, the latest state, the command, and the
   health in one transaction. A repeated event ID changes nothing.
4. The **policy** produces a command, but only for a fresh accepted event.
   Late or historical telemetry never commands a battery.
5. The run **continues** to the next interval, whatever happened, until it is
   told to stop — by Ctrl+C, or by a configured interval count.

Latest state is ordered by event time, never by arrival. Old telemetry is kept
as history but can never overwrite newer state.

## What is deliberately not here

No forecasts: the profiles are synthetic and deterministic. The battery model
assumes perfect efficiency and no power limit. The grid absorbs any surplus and
supplies any deficit. There is no network API, no cloud service, and no real
hardware. Each of those arrives in a later milestone, or not at all.

## Documentation

- [Setup](docs/SETUP.md) — install and run
- [Demo](docs/DEMO.md) — the two scenarios, line by line
- [Architecture](docs/ARCHITECTURE.md) — components and data flow
- [Model](docs/MODEL.md) — the energy and battery model
- [Decision records](docs/engineering/adr/) — why the system is built this way
- [Roadmap](docs/roadmap.md) — what is done and what comes next

## License

Wattfeder is available under the [MIT License](LICENSE).
