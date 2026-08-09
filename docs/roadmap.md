# Wattfeder Roadmap

Wattfeder is a Go project for exploring distributed energy software. Its target
system is described in [`engineering/GOALS.md`](engineering/GOALS.md): a Go edge
agent that processes household telemetry locally, keeps working while the cloud
is unreachable, and delivers events reliably to an ASP.NET Core ingestion
service backed by PostgreSQL.

This roadmap sequences those goals. `GOALS.md` states what the finished system
must demonstrate; this file states in which order it is built and what counts as
finished at each step.

Each milestone should leave a working system before more infrastructure or
domain behavior is added.

The current milestone should remain the main focus. Later milestones describe
possible directions rather than fixed commitments.

## Progress

| Milestone | Status |
| --- | --- |
| v0.1 — Single Household Simulation | Complete |
| v0.2 — Persistent Device State | Complete |
| v0.3 — Unreliable Telemetry | Current |
| v0.4 — Single-Site Edge Runtime | Planned |
| v0.5 — Observability and Local Operations | Planned |
| v0.6 — Cloud Ingestion Service | Planned |
| v0.7 — Offline-Capable Edge Delivery | Planned |
| v0.8 — Fleet Simulation and Ingestion Load | Planned |
| v0.9 — Azure Deployment with Pulumi | Planned |
| v1.0 — Portfolio Readiness | Planned |

The application now persists telemetry, commands, and latest device state in
SQLite, restores battery state after restart, rejects duplicate durable events,
and commits each processing result before applying its command. The current
milestone defines behavior for unreliable telemetry.

Everything up to and including v0.5 builds one edge agent. v0.6 introduces the
cloud boundary, v0.7 makes delivery survive disconnection, v0.8 measures the
fleet against the cloud service, and v0.9–v1.0 make the result deployable and
reviewable.

## Deployment and Fleet Model

Wattfeder deploys one edge-agent instance per household or energy site.

Each agent:

* manages one logical household energy system;
* owns an independent SQLite database;
* processes local telemetry and control decisions;
* continues operating without cloud connectivity;
* uploads durable telemetry to the shared cloud ingestion service.

A fleet is a collection of independent edge agents connected to the same cloud
service. Fleet-wide concurrency, partitioning, throughput and load management
are cloud-side concerns.

A future site adapter may combine readings from several local assets, such as a
PV inverter, battery and smart meter. This does not make the edge agent a
multi-household fleet gateway.

### Identity semantics

```text
agent_id  = one installed edge-agent instance
device_id = the logical household energy system managed by that agent
event_id  = one immutable telemetry event
```

Do not introduce `site_id`, `asset_id` or a physical-device registry yet. Add
them when a real multi-device adapter requires them.

## Standing constraint: domain independence

Control policy and device-state logic must not depend on transport, database, or
cloud services. Telemetry reaches the domain through an adapter, and durable
storage and delivery sit behind interfaces the domain owns.

This constraint holds for every milestone rather than belonging to one. It is
what keeps the simulator, an HTTP client, SQLite, and PostgreSQL replaceable, and
it is the reason the disposition rules defined in v0.3 can be reused unchanged by
the cloud projection in v0.6.

The constraint is verified rather than asserted: v0.4 introduces the
`TelemetrySource` and `CommandSink` contracts and an import check that fails when
a domain package imports a transport or storage package, and v0.6 and v0.7 keep
that check passing as HTTP and PostgreSQL arrive.

## Standing constraint: bounded work and explicit overload

Queues are not added to satisfy the word "bounded". They are added where a real
producer can outrun a real consumer, and every bound has a defined behavior when
it is reached.

At the edge:

* The SQLite outbox is the durable backlog. A bounded in-memory channel may
  decouple processing from persistence or upload, but only where it is needed.
* When an in-memory bound is reached, producers block, reject, or spill to
  durable storage according to an explicit documented rule.
* SQLite's single-writer behavior is acceptable at household telemetry rates.

In the cloud:

* ASP.NET request concurrency, database connections and transaction throughput
  are the initial capacity boundaries.
* On overload the service returns a retryable response rather than accepting
  work it cannot make durable.
* No Kafka, Service Bus or internal broker is introduced until measurement shows
  that synchronous ingestion cannot meet a defined requirement.

---

## v0.1 — Single Household Simulation

**Status:** Complete. The implementation and its automated checks are the
baseline for later milestones.

### Goal

Build the smallest complete system that processes simulated household energy
data and produces deterministic control decisions.

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
Validated in-memory state
        ↓
Control policy
        ↓
Command
        ├──→ Structured output
        └──→ Battery state for the next interval
```

### Example output

```json
{
  "timestamp": "2026-08-07T00:00:00Z",
  "device_id": "home-001",
  "pv_power_kw": 0,
  "load_power_kw": 0.3861461516439471,
  "battery_soc_percent": 50,
  "electricity_price_eur_kwh": 0.31498997331311535,
  "decision": "discharge",
  "command_power_kw": 0.3861461516439471,
  "reason": "Electricity price is at or above EUR 0.30/kWh and household load exceeds PV production"
}
```

### Completion evidence

* `go run ./cmd/wattfeder` runs one configurable simulated day and emits
  newline-delimited JSON.
* Repeat runs with the same configuration and seed produce identical output.
* Every command contains a charge, discharge, or idle decision and a
  human-readable reason.
* Telemetry and commands reject blank, non-finite, out-of-range, and
  structurally invalid values without partially updating state.
* Table-driven tests cover policy thresholds, battery bounds, and limiting a
  discharge so it cannot cross the 20% reserve during an interval.
* Application tests cover cancellation and failures while obtaining telemetry,
  applying commands, and writing output.
* CLI tests cover help, invalid arguments, configuration mapping, deterministic
  output, cancellation, and output errors.
* `make demo` loads a fixed JSON scenario, exposes each telemetry and decision
  step, and verifies the expected decision sequence.
* `make check` verifies formatting, module metadata, static analysis, tests, and
  compilation.
* The setup, demo, architecture, and model guides document execution,
  configuration, current behavior, and verification.

### Deliberate limitations

* One process simulates one household for one 24-hour window.
* Telemetry, latest state, decisions, and output are not persisted.
* Profiles are synthetic and deterministic rather than forecasts.
* The battery model assumes perfect efficiency and no battery power limit.
* The grid implicitly absorbs unused photovoltaic surplus and supplies demand
  not served by the battery.
* There is no network API, external message broker, database, or real-device
  integration.

---

## v0.2 — Persistent Device State

**Status:** Complete. SQLite persistence, restart restore, atomic processing,
and duplicate protection are implemented and verified.

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

### Completion evidence

* The CLI owns a configurable SQLite database path, applies ordered migrations,
  and restores the latest battery SOC before constructing the simulator.
* Telemetry, its command, and latest state commit atomically before the command
  advances the simulator.
* Duplicate event IDs change no records and stop without redelivering a command.
* Application and CLI tests cover restart restore, duplicate processing,
  persistence-failure ordering, migration, rollback, and traceability.

---

## v0.3 — Unreliable Telemetry

**Status:** Current. Failure semantics and deterministic fault cases are the
next implementation focus.

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
* An explicit distinction between event time and receive time.

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

Record the result as a disposition matrix covering every supported case. The
same matrix is later reused by the cloud ingestion service, so its semantics
must not depend on where processing happens.

### Ordering rule

Old telemetry may be stored as history, but it must never replace a newer latest
state. This rule is the local half of the guarantee the cloud projection repeats
in v0.6.

### Exit criteria

* Every supported failure mode has defined behavior.
* Every supported failure mode has at least one automated test.
* Faults can be reproduced deterministically.
* A rejected or ignored event does not stop the agent from processing the events
  that follow it. Isolation between separate energy systems is a cloud-side
  concern from v0.6 onward, because one agent manages one of them.
* Logs explain why an event was rejected or ignored.
* A delayed event cannot overwrite a newer latest state.
* The disposition matrix is documented and matches the implemented behavior.

### Engineering records

* ADR — disposition of duplicate, out-of-order, delayed, invalid, and incomplete
  telemetry, including the ordering key and the event-time/receive-time split.
* ADR — store old telemetry without allowing stale latest-state updates.

---

## v0.4 — Single-Site Edge Runtime

### Goal

Turn the current finite simulation runner into a long-running edge-agent runtime
for one household or energy site.

### Scope

* Run one edge-agent instance per household.
* Separate telemetry production from edge processing.
* Introduce `TelemetrySource` and `CommandSink` contracts.
* Support the deterministic simulator through those contracts.
* Replace the fixed one-day application loop with context-driven processing.
* Preserve event ordering for the local household stream.
* Continue after duplicates and individually rejected events.
* Keep SQLite ownership inside the edge runtime.
* Support graceful shutdown without losing accepted events.
* Bound any in-memory handoff between telemetry processing and persistence.

### Explicit non-goals

* processing multiple households in one agent;
* fleet-wide partitioning;
* cloud-ingestion load testing;
* running hundreds of agent processes;
* optimizing SQLite for high-frequency multi-tenant workloads.

### Engineering questions

* Where does the runtime loop end and the domain begin?
* Is the ordering key the event time, the receive time, or the arrival sequence?
* Which in-memory handoff genuinely needs a bound, and what happens when it is
  reached?
* What must be true before an accepted event may be dropped during shutdown?
* How is one agent's identity configured and kept distinct from another's?

### Exit criteria

* One agent processes one household continuously.
* Two agent instances can run independently with different identities and SQLite
  databases.
* The simulator is an adapter rather than a dependency of the edge runtime.
* A duplicate or invalid event does not terminate subsequent processing.
* Shutdown does not abandon an event already accepted for persistence.
* No queue or goroutine can grow without a configured bound.
* No domain package imports a transport or storage package, enforced by an
  automated import check.

### Engineering records

* ADR — keep the simulator as a deterministic telemetry adapter behind the
  `TelemetrySource` contract.
* ADR — one edge agent per household; fleet scaling belongs to cloud ingestion.
* ADR — SQLite single-writer throughput is acceptable for one household agent.
  Revisit when one agent must process multiple independent sites, telemetry
  frequency exceeds measured local capacity, or persistence latency interferes
  with control processing.

---

## v0.5 — Observability and Local Operations

### Goal

Make the running edge agent understandable and diagnosable before it is
connected to a cloud service.

### Scope

* Structured logging.
* Prometheus metrics.
* OpenTelemetry instrumentation of local processing.
* Health endpoint.
* Readiness endpoint.
* Graceful shutdown.
* Processing latency metrics.
* Event lag metrics.
* Device health metrics.
* Error and rejection counters.
* Docker image for the edge agent.
* Local Docker Compose environment.
* Small operational runbook.

### Example metrics

```text
wattfeder_telemetry_received_total
wattfeder_telemetry_processed_total
wattfeder_telemetry_rejected_total
wattfeder_commands_created_total
wattfeder_device_health          # one gauge labeled by health state
wattfeder_processing_duration_seconds
wattfeder_event_lag_seconds
wattfeder_queue_depth
```

An agent reports the health of the one household energy system it manages. Gauges
that count online, stale and offline devices across a fleet belong to the
ingestion service in v0.6, because only the cloud can see more than one agent.

### Operational questions

* How can an operator tell whether telemetry is still arriving?
* How can an operator identify delayed processing?
* How can an operator tell that this agent's household system is unhealthy?
* How does the system behave during shutdown?
* Which state is lost when the process stops?
* Which failures should affect readiness?

### Exit criteria

* `docker compose up` starts the edge agent and its local dependencies.
* Health and readiness checks have different documented purposes.
* Key behavior is visible through metrics.
* Logs contain enough context to trace an event.
* One local processing run produces a trace that can be inspected.
* The process shuts down without silently abandoning accepted work.
* The runbook describes common failure scenarios.

---

## v0.6 — Cloud Ingestion Service

### Goal

Introduce the cloud boundary: an ASP.NET Core service that accepts telemetry
batches over HTTP and stores them durably in PostgreSQL.

### Scope

* ASP.NET Core ingestion service.
* PostgreSQL schema and migrations.
* An OpenAPI-described batch ingestion endpoint.
* Idempotent processing keyed by event ID.
* Telemetry history separate from a latest-state projection.
* Per-event outcomes in the batch response.
* Health and readiness endpoints with different semantics.
* Correct behavior when many independent agents deliver concurrently, including
  idempotency across agents and per-device projections that do not interfere.
* Structured logs and metrics for ingestion, including the fleet-wide gauges for
  online, stale and offline agents that no single agent can report.
* Integration tests against a real PostgreSQL instance.
* Cloud reuse of the v0.3 disposition rules, so an event is judged the same way
  at the edge and in the cloud.

### Contract sketch

```text
POST /v1/telemetry:batch
        ↓
per-event outcome
        ├──→ accepted
        ├──→ duplicate
        ├──→ rejected (invalid)
        └──→ stored as history only (older than latest state)
```

### Engineering questions

* What is the smallest useful ingestion contract?
* Which validation belongs at the boundary and which belongs in the domain?
* How is a duplicate distinguished from a retry of a partially applied batch?
* What does a mixed-quality batch return, and is it a success or a failure?
* Is a batch applied atomically, or per event?
* Which failures must make readiness fail instead of returning success?

### Exit criteria

* A telemetry batch can be ingested and read back from PostgreSQL.
* Submitting the same batch twice produces one durable row per event.
* An invalid event in a batch cannot corrupt the latest-state projection.
* An older event is retained as history without replacing a newer projection.
* The response explains the outcome of every event in the batch.
* Readiness fails while PostgreSQL is unavailable, and no request receives a
  false acknowledgement.
* The OpenAPI description matches the implemented behavior.
* Integration tests run against real PostgreSQL, not an in-memory substitute.
* One agent sending invalid or duplicate data cannot affect ingestion for any
  other agent.
* The import check from v0.4 still passes with HTTP and PostgreSQL present.
* Overload returns a retryable response instead of an acknowledgement the
  service cannot make durable.

### Engineering records

* ADR — PostgreSQL for centralized persistence.
* ADR — HTTP batches before introducing a message broker.

---

## v0.7 — Offline-Capable Edge Delivery

### Goal

Make the edge agent keep working while the cloud is unavailable, and deliver
every accepted event once the cloud returns.

### Scope

* A durable SQLite outbox written in the same transaction as the processing
  result.
* An uploader with bounded batches, timeouts, retries and backoff.
* At-least-once delivery against idempotent cloud processing.
* Delivery state per event.
* Backlog drain after reconnection.
* Retention and storage-limit behavior for the local database.
* Outbox depth and delivery metrics.
* A generated or hand-written client for the ingestion contract.
* Contract tests between the Go client and the .NET API.
* A Docker Compose environment containing one edge agent, the ingestion service
  and PostgreSQL.
* OpenTelemetry trace context propagated from edge processing into cloud
  ingestion, with correlated logs, metrics and traces.
* Scripted single-agent scenarios for offline backlog recovery, lost
  acknowledgement, and mixed-quality batches.

### Delivery flow

```text
Telemetry event
        ↓
Local processing + outbox entry (one transaction)
        ↓
Local command applied
        ↓
Uploader batch
        ↓
Cloud acknowledgement
        ↓
Outbox entry marked delivered
```

### Engineering questions

* When is an event safe to mark delivered?
* What happens when an acknowledgement is lost after a successful commit?
* How large may a batch be, and what bounds the retry interval?
* What happens when local storage reaches its configured limit?
* Which events may be discarded, and which must never be?
* Does upload failure ever block local control decisions?

### Exit criteria

* Local control continues unchanged while the cloud is unreachable.
* Pending events survive a process restart.
* Reconnection drains the backlog without creating duplicate cloud rows.
* A lost acknowledgement leads to a retry, not to a lost or duplicated event.
* An event is marked delivered only after durable cloud persistence.
* The behavior at the local storage limit is documented and tested, not
  implicitly unbounded.
* Contract tests fail when the Go client and the .NET API disagree.
* The outbox and uploader are reachable from the domain only through interfaces
  the domain defines, and the import check from v0.4 still passes.
* Removing the uploader leaves local control compiling and working.
* `docker compose up` starts one agent, the ingestion service and PostgreSQL
  with one documented command.
* One event can be traced from the simulator through SQLite into PostgreSQL.
* Scenarios 1 to 3 from [`engineering/GOALS.md`](engineering/GOALS.md) run from
  documented commands, and each guarantee they demonstrate is also covered by an
  automated test.

### Engineering records

* ADR — at-least-once delivery with idempotent processing.
* ADR — SQLite for edge durability and the offline outbox.
* ADR — keep control decisions local at the edge.

---

## v0.8 — Fleet Simulation and Ingestion Load

### Goal

Measure how the cloud ingestion service behaves when many independent household
agents upload telemetry concurrently.

This milestone follows v0.7 rather than v0.6 because the load model needs an
outbox, a backlog and a reconnect path to emulate. Before those exist, a fleet
test measures nothing that resembles the real system.

### Load model

Use a dedicated load generator that emulates independent agents. Do not start
hundreds of operating-system processes merely to produce HTTP traffic.

Each virtual agent must have:

* a stable agent and device identity;
* its own event sequence;
* configurable telemetry frequency;
* configurable offline backlog;
* optional duplicate and stale events;
* reconnect jitter.

A small end-to-end run of three to ten real edge-agent containers remains useful
for verifying the real binary. The load test itself uses virtual agents in one
controlled generator, so it measures cloud behavior rather than container
startup overhead.

### Required scenarios

1. Steady-state ingestion from many agents.
2. Agents reconnecting after a shared outage.
3. Duplicate delivery after lost acknowledgements.
4. A mixed batch containing accepted, duplicate, stale and rejected events.
5. PostgreSQL slowdown or temporary unavailability.

Scenarios 4 and 5 from [`engineering/GOALS.md`](engineering/GOALS.md) are proven
here under load; v0.7 proves the single-agent semantics they build on.

### Measurements

* accepted events per second;
* p50, p95 and p99 batch latency;
* error rate;
* event lag;
* duplicate rate;
* database connection-pool usage;
* PostgreSQL transaction latency;
* CPU and memory;
* recovery time after a reconnect wave.

### Exit criteria

* The test is reproducible.
* The environment and configuration are recorded.
* Queues and concurrency are bounded.
* Overload behavior is explicit.
* At least one measured bottleneck and the resulting decision are documented.
* No unsupported production-scale claim is made.

### Incident exercise: reconnect storm

```text
500 simulated household agents lose cloud connectivity
→ each accumulates a local backlog
→ connectivity returns
→ agents retry with jitter and bounded batches
→ ingestion slows
→ PostgreSQL connection pressure increases
→ no events are falsely acknowledged
→ backlog eventually drains
```

This exercise is the natural place to examine backoff and jitter,
thundering-herd prevention, database capacity, per-agent fairness, event lag,
idempotency, overload responses, and recovery time.

If the first implementation reconnects every agent simultaneously and overwhelms
PostgreSQL, that becomes a genuine engineering postmortem: document the measured
failure, its root cause, the corrective change, and the regression scenario that
proves it cannot silently recur. Label it an exercise. Do not write a fictional
production incident.

---

## v0.9 — Azure Deployment with Pulumi

### Goal

Deploy and operate the ingestion service in Azure using reproducible
infrastructure code.

### Scope

* Pulumi program describing the cloud infrastructure.
* Azure Container Apps hosting for the ingestion service.
* Managed PostgreSQL.
* Container registry and image publication.
* Configuration and secrets held outside images and source control.
* Readiness-driven revision rollout.
* Resource limits based on observed usage.
* Deployment and teardown documentation.

### Explicitly out of scope

The following are excluded deliberately, not for lack of time:

* Kubernetes and AKS,
* Kafka or a managed message broker,
* Cosmos DB,
* Service Bus,
* additional microservices,
* multi-region operation,
* automatic horizontal scaling without measured need.

Each exclusion should have a documented condition that would justify revisiting
it.

### Operational exercises

* Deploy a new revision while events are being uploaded.
* Restart the ingestion service during an upload.
* Deploy an invalid configuration.
* Make the database temporarily unavailable.
* Exhaust a configured resource limit.

### Exit criteria

* The ingestion service can be deployed and destroyed with documented commands.
* A redeployment does not create duplicate cloud rows.
* Readiness prevents traffic from reaching an instance without a database.
* No secret or environment-specific value is stored in an image or in the
  repository.
* Resource configuration is justified by the fleet load test from v0.8, not by
  guesswork.
* Cloud hosting is documented as a deployment choice rather than part of the
  domain.

### Engineering records

* ADR — Azure Container Apps instead of AKS.
* ADR — Pulumi for reviewable, reproducible infrastructure.

---

## v1.0 — Portfolio Readiness

### Goal

Make the system reviewable by another engineer without assistance.

### Scope

* One concise architecture overview linking decisions to code and verification.
* A complete ADR set covering the major trade-offs.
* Postmortems for the failures actually encountered.
* Documented commands for every scenario and test suite.
* Documentation that describes the system that exists.

### Exit criteria

Wattfeder is portfolio-ready when another engineer can:

1. understand the architecture in a few minutes;
2. start the complete local system with one documented command;
3. run the important failure scenarios;
4. trace one event from the simulator through SQLite into PostgreSQL;
5. inspect the relevant logs and metrics;
6. understand the major trade-offs through ADRs;
7. verify the claimed guarantees through automated tests;
8. deploy the ingestion service through documented infrastructure code.

The project is not complete merely because all planned features exist. It is
complete when its most important reliability claims are demonstrated and easy to
review.

---

## Engineering Records

ADRs and postmortems are written alongside the milestone that produces them, not
collected at the end.

An ADR is worth writing when a decision had at least two credible alternatives
and a real downside. Each one records context, decision, rejected alternatives,
consequences, and the measurable condition that would justify revisiting it.

Records already justified by completed work:

* atomically persist each processing result before applying its command (v0.2);
* producer-owned event IDs as the basis for idempotent processing (v0.2);
* embedded SQLite for the single-process persistence milestone (v0.2).

These three are documented retrospectively and should be marked as such.

Remaining records are listed under the milestone that forces the decision. The
recommended set in [`engineering/GOALS.md`](engineering/GOALS.md) is the starting
point, extended by the two records the deployment model requires: one edge agent
per household with fleet scaling in the cloud, and SQLite single-writer
throughput as sufficient for one household agent. Numbering is assigned when a
record is written.

Postmortems describe failures that were actually encountered during
implementation, testing, or a controlled incident exercise. A planned exercise
without an unexpected finding is a test report, not a postmortem. Production
incidents are never invented, and simulated failures are labeled as exercises.

---

## Deferred Directions

These were earlier roadmap milestones. They are deferred because they do not
serve the edge-to-cloud goal, not because they were rejected. At most one should
become the primary direction after v1.0.

### Track A — Pluggable telemetry sources

A second source behind the `TelemetrySource` contract established in v0.4: CSV
replay with
preserved event order, configurable replay speed, deterministic fault injection,
and documented handling of malformed rows. This track adds a source; it does not
create the boundary, which the standing domain-independence constraint already
requires.

### Track B — Device fleet management

Device registration, metadata, heartbeats, agent versions, desired and reported
configuration, command delivery status, acknowledgements, and reconciliation
after reconnecting. This turns the downstream command path into the mirror image
of the upstream delivery path built in v0.7.

### Track C — Energy management

Dynamic electricity prices, battery constraints, photovoltaic and consumption
forecasts, dispatch schedules, cost comparison, and explainable optimization
decisions. This track requires more energy-domain knowledge and should follow a
reliable base system.

### Track D — Device integrations

A normalized device model with adapter boundaries for Modbus TCP, SunSpec and
MQTT, including vendor differences, retries, timeouts, and capability discovery.
Real equipment should initially be integrated read-only.

---

## Development Principles

### Working software before infrastructure

A working vertical slice has priority over message brokers, dashboards or cloud
deployment.

### Reliability before optimization

The system should first behave correctly during duplicates, restarts and
delayed events. Performance optimization should follow measurement.

### Every technology solves a visible problem

A component is added only when a concrete reliability, operational or domain
problem requires it. The goal is engineering judgment, not the largest possible
technology stack.

### Current architecture only

Architecture documentation should describe the system that exists. Planned
architecture belongs in this roadmap.

### Explicit behavior

Failures, invalid data and overload must have defined behavior. They should not
be handled implicitly.

### Reproducibility

Simulations, tests, benchmarks and fault scenarios should be reproducible.

### Explainable decisions

Every generated control decision should include enough information to understand
why it was made.

### Evidence over claims

A guarantee counts as demonstrated when a test, measurement or scenario proves
it. Performance and reliability claims include the conditions under which they
were observed.

### Limited scope

Each release should solve one main engineering problem. Features that do not
contribute to its exit criteria should normally be postponed.

---

## Current Focus

The current focus is:

```text
v0.3 — Unreliable Telemetry
```

The next implementation sequence is:

1. Define accept, reject, history-only, and state-update behavior for each
   unreliable telemetry case, and record it as a disposition matrix.
2. Separate event time from receive time so delayed events remain orderable.
3. Reject or isolate out-of-order, delayed, missing, and invalid measurements
   without corrupting the latest valid state.
4. Add deterministic simulator fault injection for every supported failure.
5. Introduce online, stale, offline, and invalid device health states.
6. Make failure decisions observable with structured reasons.
7. Cover every supported failure mode with automated tests.

The long-running edge runtime, the cloud ingestion service, the offline outbox,
distributed tracing, the fleet load test, and Azure deployment remain outside the
current milestone. Kubernetes, Kafka, a frontend,
and real hardware remain outside the project unless a measured requirement makes
them necessary.
