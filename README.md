# Wattfeder

Wattfeder is a deterministic Go simulation of household energy flows and
battery control. It generates photovoltaic production, household load,
electricity price, and battery telemetry for one 24-hour period, then chooses a
charge, discharge, or idle command for each interval.

The current version runs one household locally, validates telemetry, evolves
battery state, and emits newline-delimited JSON. A SQLite adapter can migrate
and atomically store processing records, but the CLI still keeps active state in
memory and does not open a database yet.

## Run

Wattfeder requires Go 1.26.5. Go downloads the pure-Go SQLite dependency when
needed.

```bash
make demo   # Run the fixed four-step scenario
make run    # Run the configurable 24-hour simulation
make check  # Format, analyze, test, and build the project
```

Run `go run ./cmd/wattfeder -help` to list the available simulation flags.

## Model boundaries

Profiles are synthetic and deterministic, not forecasts. The battery model
assumes perfect efficiency and no power limit; the grid supplies unmet demand
and absorbs unused photovoltaic generation.

## Documentation

- [Setup](docs/SETUP.md)
- [Demo](docs/DEMO.md)
- [Architecture](docs/ARCHITECTURE.md)
- [Model](docs/MODEL.md)
- [Roadmap](docs/roadmap.md)

## License

Wattfeder is available under the [MIT License](LICENSE).
