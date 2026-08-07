# Wattfeder Roadmap

Wattfeder is a learning and portfolio project for exploring reliable distributed energy software in Go.

The roadmap is intentionally incremental. Each milestone should produce a working, demonstrable system before additional infrastructure or domain complexity is introduced.

The current milestone should remain the main focus. Later milestones describe possible directions rather than fixed commitments.

## Progress

| Milestone | Status |
| --- | --- |
| v0.1 — Single Household Simulation | In progress |
| v0.2–v0.7 | Planned |

The deterministic simulator, battery state evolution, telemetry validation,
and latest in-memory state for one household device are complete. Control
decisions, application output, and graceful shutdown still remain for v0.1.

---

## v0.1 — Single Household Simulation

### Goal

Build the smallest complete system that processes simulated household energy data and produces deterministic control decisions.

### Scope

* Simulate one household.
* Simulate photovoltaic generation.
* Simulate household electricity consumption.
* Model a battery with a state of charge.
* Provide a simulated electricity price.
* Process telemetry at a fixed interval.
* Produce one of three decisions:

  * charge
  * discharge
  * idle
* Include a human-readable reason with every decision.
* Validate incoming telemetry.
* Handle graceful shutdown.
* Add table-driven tests for the control policy.

### Data flow

```text
Household simulator
        ↓
Telemetry event
        ↓
State update
        ↓
Control policy
        ↓
Command
        ↓
Structured output
```

### Example output

```text
time=2026-08-03T14:00:00Z
device=home-001
pv_power_kw=4.8
load_power_kw=1.9
battery_soc=61
electricity_price_eur_kwh=0.28
decision=charge
power_kw=2.9
reason="PV production exceeds household load"
```

### Exit criteria

* `go run ./cmd/wattfeder` starts the simulation.
* The same input always produces the same decision.
* Decisions include an understandable explanation.
* Invalid telemetry is rejected or handled explicitly.
* Relevant policy boundaries are covered by tests.
* Another developer can clone and run the project using the README.

---

## v0.2 — Persistent Device State

### Goal

Preserve device state and decision history across process restarts.

### Scope

* Add SQLite persistence.
* Store received telemetry events.
* Store the latest known state of each device.
* Store generated commands.
* Assign a unique ID to every telemetry event.
* Restore the latest state after startup.
* Introduce simple database migrations.
* Define transaction boundaries.
* Prevent duplicate event processing.

### Questions to answer

* What state must survive a restart?
* How is an event identified uniquely?
* When is an event considered processed?
* Can telemetry and its resulting command be persisted atomically?
* What happens when persistence fails after receiving an event?

### Exit criteria

* Restarting the process restores the latest device state.
* Processing the same event twice does not create duplicate commands.
* Telemetry and resulting decisions remain traceable.
* Database setup and migrations are reproducible.
* Persistence behavior is covered by integration tests.

---

## v0.3 — Unreliable Telemetry

### Goal

Handle common failure modes of distributed and intermittently connected devices.

### Scope

* Duplicate events.
* Out-of-order events.
* Delayed events.
* Missing values.
* Invalid measurements.
* Missing heartbeats.
* Temporarily unavailable devices.
* Slow telemetry producers.
* Configurable fault injection in the simulator.

### Device health states

```text
online
stale
offline
invalid
```

### Example failure cases

```text
battery SOC above 100 percent
event timestamp older than current state
event ID received more than once
device sends no heartbeat for 90 seconds
telemetry arrives several minutes late
```

### Required design decisions

For each failure case, document whether the system:

* accepts the event,
* rejects the event,
* stores it without applying it,
* updates historical data only,
* marks the device as unhealthy,
* or retries an operation.

### Exit criteria

* Every supported failure mode has defined behavior.
* Every supported failure mode has at least one automated test.
* Faults can be reproduced deterministically.
* One faulty device cannot stop processing for all other devices.
* Logs explain why an event was rejected or ignored.

---

## v0.4 — Multi-Device Processing

### Goal

Expand the system from one household to a small fleet of simulated devices.

### Scope

* Simulate at least 100 households.
* Generate different PV and consumption profiles.
* Process device updates concurrently.
* Preserve ordering where ordering matters.
* Introduce bounded queues.
* Handle slow consumers.
* Measure throughput and processing latency.
* Add a reproducible load-test command.

### Engineering questions

* Does processing require ordering per device?
* Which operations can run concurrently?
* What happens when incoming load exceeds processing capacity?
* Should events be dropped, rejected or buffered?
* How much state is stored per device?
* Where does contention occur?

### Measurements

Record at least:

* events processed per second,
* median processing latency,
* high-percentile processing latency,
* memory consumption,
* CPU consumption,
* queue depth,
* rejected or dropped events.

### Exit criteria

* A reproducible load test processes at least 100 simulated households.
* The system uses bounded concurrency.
* Overload behavior is explicit.
* CPU and memory profiles have been inspected.
* At least one measured bottleneck and its resolution are documented.
* Performance claims include the test environment and configuration.

---

## v0.5 — Observability and Local Operations

### Goal

Make the running system understandable and diagnosable.

### Scope

* Structured logging.
* Prometheus metrics.
* Health endpoint.
* Readiness endpoint.
* Graceful shutdown.
* Processing latency metrics.
* Event lag metrics.
* Device health metrics.
* Error and rejection counters.
* Docker image.
* Local Docker Compose environment.
* Small operational runbook.

### Example metrics

```text
wattfeder_telemetry_received_total
wattfeder_telemetry_processed_total
wattfeder_telemetry_rejected_total
wattfeder_commands_created_total
wattfeder_devices_online
wattfeder_devices_stale
wattfeder_devices_offline
wattfeder_processing_duration_seconds
wattfeder_event_lag_seconds
wattfeder_queue_depth
```

### Operational questions

* How can an operator tell whether telemetry is still arriving?
* How can an operator identify delayed processing?
* How can an operator find one unhealthy device?
* How does the system behave during shutdown?
* Which state is lost when the process stops?
* Which failures should affect readiness?

### Exit criteria

* `docker compose up` starts the complete local environment.
* Health and readiness checks have different documented purposes.
* Key behavior is visible through metrics.
* Logs contain enough context to trace an event.
* The process shuts down without silently abandoning accepted work.
* The runbook describes common failure scenarios.

---

## v0.6 — Pluggable Telemetry Sources

### Goal

Separate telemetry processing from the source that supplies the data.

### Scope

Implement a shared telemetry-source abstraction with at least:

* simulator source,
* CSV replay source.

Possible later sources:

* public energy dataset,
* HTTP source,
* MQTT source,
* read-only inverter adapter.

### Example commands

```bash
wattfeder run --source simulator
wattfeder run --source csv --file ./data/sample.csv
```

### CSV replay behavior

* Preserve the original event order.
* Support configurable replay speed.
* Support deterministic fault injection.
* Preserve original timestamps or map them to replay time.
* Document how malformed rows are handled.

### Architectural constraint

Control policies and device-state logic must not depend on the concrete telemetry source.

### Exit criteria

* The same processing pipeline works with simulation and CSV replay.
* Adding a source does not require changing the control policy.
* Historical data can be replayed faster than real time.
* Source-specific failures are isolated from domain logic.
* Source contracts are covered by tests.

---

## v0.7 — Device Fleet Management

### Goal

Explore infrastructure for managing a fleet of intermittently connected energy devices.

### Scope

* Device registration.
* Device metadata.
* Heartbeats.
* Agent or firmware version.
* Desired configuration.
* Reported configuration.
* Device health state.
* Command delivery status.
* Command acknowledgements.
* Recovery after reconnecting.

### Example state flow

```text
Desired configuration
        ↓
Command created
        ↓
Device receives command
        ↓
Device applies command
        ↓
Reported configuration updated
```

### Engineering questions

* How are commands identified?
* Can commands be safely retried?
* What happens when acknowledgements are lost?
* How are desired and reported state reconciled?
* When does an offline device receive pending changes?
* How is an outdated device version identified?

### Exit criteria

* A device can register and send heartbeats.
* Offline devices are detected.
* Configuration changes are represented as desired state.
* Simulated devices acknowledge applied commands.
* Reconnection triggers state reconciliation.
* Duplicate command delivery does not produce unsafe repeated actions.

---

## v0.8 — Kubernetes Deployment

### Goal

Deploy and operate the existing system in a local Kubernetes environment.

### Scope

* Local cluster using `kind`.
* Deployment.
* Service.
* ConfigMap.
* Secret.
* Readiness probe.
* Liveness probe.
* Resource requests.
* Resource limits.
* Rolling update.
* Persistent volume where required.
* Basic deployment documentation.

### Explicitly out of scope

* service mesh,
* multi-cluster operation,
* custom Kubernetes operator,
* production-grade cloud infrastructure,
* complex Helm abstractions,
* automatic horizontal scaling without measured need.

### Operational exercises

* Restart a pod.
* Terminate a pod while events are being processed.
* Perform a rolling update.
* Deploy an invalid configuration.
* Exhaust a configured resource limit.
* Temporarily make the database unavailable.

### Exit criteria

* The system can be deployed using documented commands.
* Pod restarts do not create duplicate commands.
* Readiness prevents traffic from reaching an unavailable instance.
* A rolling update completes without corrupting persistent state.
* Resource configuration is based on observed usage.
* Kubernetes is documented as a deployment choice rather than part of the domain.

---

## Possible Specialization Tracks

These tracks are optional. Only one should become the primary direction.

### Track A — Device Infrastructure

Focus areas:

* large device fleets,
* heartbeats,
* configuration distribution,
* software versions,
* remote diagnostics,
* offline operation,
* reconnect behavior,
* edge-agent lifecycle.

This track is most relevant for device-platform and energy-IoT infrastructure roles.

### Track B — Energy Management

Focus areas:

* dynamic electricity prices,
* battery constraints,
* photovoltaic forecasts,
* consumption forecasts,
* dispatch schedules,
* cost comparison,
* explainable optimization decisions.

This track requires more energy-domain knowledge and should follow a reliable base system.

### Track C — Device Integrations

Focus areas:

* normalized device model,
* adapter boundaries,
* Modbus TCP,
* SunSpec,
* MQTT,
* vendor-specific differences,
* retries and timeouts,
* capability discovery.

Real equipment should initially be integrated read-only.

---

## Development Principles

### Working software before infrastructure

A working vertical slice has priority over Kubernetes, message brokers, dashboards or cloud deployment.

### Reliability before optimization

The system should first behave correctly during duplicates, restarts and delayed events. Performance optimization should follow measurement.

### Current architecture only

Architecture documentation should describe the system that exists. Planned architecture belongs in this roadmap.

### Explicit behavior

Failures, invalid data and overload must have defined behavior. They should not be handled implicitly.

### Reproducibility

Simulations, tests, benchmarks and fault scenarios should be reproducible.

### Explainable decisions

Every generated control decision should include enough information to understand why it was made.

### Limited scope

Each release should solve one main engineering problem. Features that do not contribute to its exit criteria should normally be postponed.

---

## Current Focus

The current focus is:

```text
v0.1 — Single Household Simulation
```

The immediate implementation sequence is:

1. Define telemetry and command models.
2. Implement deterministic household simulation.
3. Implement a minimal control policy.
4. Add table-driven policy tests.
5. Add input validation.
6. Add structured console output.
7. Add graceful shutdown.
8. Document how to run the simulation.

No persistence, Kubernetes, frontend, real hardware or external message broker is required for v0.1.
