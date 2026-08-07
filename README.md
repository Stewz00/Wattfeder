Wattfeder is a learning and portfolio project for exploring reliable
distributed energy systems in Go.

It is not a production virtual power plant. The project focuses on
telemetry processing, device state, control decisions, failure handling,
observability, and deployment.

## Current status

Milestone v0.1 is in progress. The simulator currently produces one
deterministic 24-hour UTC timeline containing photovoltaic (PV) power,
household load, electricity price, and a fixed battery state of charge. The
battery model, control policy, and runnable application pipeline are not wired
yet.

Run the automated checks with:

```bash
go test ./...
```

## Documentation

- [Roadmap](docs/roadmap.md) shows current progress and planned milestones.
- [Simulation model](docs/simulation-model.md) explains the energy units,
  generated profiles, guarantees, and deliberate simplifications.
