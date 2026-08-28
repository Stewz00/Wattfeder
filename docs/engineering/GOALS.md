# Project Goals

Wattfeder is a portfolio project for demonstrating backend, distributed-systems,
edge and cloud-native engineering through a realistic household-energy system.

The project should show engineering judgment, not the largest possible technology
stack. Every component must solve a concrete reliability, operational or domain
problem.

## Primary Goal

Build a small but complete edge-to-cloud system in which a Go edge agent processes
household energy telemetry locally, continues operating during connectivity
failures, and reliably delivers events to a Go ingestion service backed by
PostgreSQL.

The finished system should be understandable, reproducible, testable and operable
by another engineer.

## What the Project Must Demonstrate

### 1. Clear system boundaries

- The simulator produces deterministic device behavior.
- The Go edge agent owns local processing, state and offline delivery.
- The Go ingestion service owns cloud ingestion and centralized persistence.
- SQLite and PostgreSQL have separate, explicit responsibilities.
- Domain code does not depend directly on transport, database or cloud services.

### 2. Reliable event processing

- Stable event identities.
- Atomic local processing and outbox creation.
- At-least-once delivery with idempotent cloud processing.
- Defined handling for duplicate, invalid, delayed and out-of-order events.
- No false acknowledgement before durable cloud persistence.
- Latest-state projections cannot be overwritten by older telemetry.

### 3. Offline-capable edge behavior

- Local control continues while the cloud is unavailable.
- Pending telemetry survives process restarts.
- Uploads use bounded batches, timeouts and backoff.
- Reconnection drains the backlog without producing duplicate cloud records.
- Local storage growth and retention behavior are explicit.

### 4. Cloud-native operation

- Containerized services.
- Reproducible local environment with Docker Compose.
- Reproducible cloud infrastructure with Terraform.
- The cloud provider is selected before v0.9 using production relevance and the
  measured Wattfeder workload rather than being coupled to the domain model.
- Health and readiness checks with different semantics.
- Structured logs, metrics and distributed tracing.
- Configuration and secrets remain outside application images and source control.

### 5. Verification through evidence

Important behavior should be demonstrated through:

- unit tests for domain rules;
- integration tests against real SQLite and PostgreSQL;
- contract tests between the edge client and ingestion API;
- end-to-end failure scenarios;
- reproducible load tests;
- documented measurements instead of unsupported performance claims.

## Authentic Technology Use

Technologies are included only where they solve a visible problem.

| Technology | Reason |
|---|---|
| Go | Long-running edge runtime, cloud ingestion, concurrency and local reliability |
| SQLite | Durable local state and offline outbox on the edge |
| PostgreSQL | Central telemetry history, idempotency and latest-state projections |
| HTTP/OpenAPI | Small, explicit and testable edge-to-cloud contract |
| OpenTelemetry | Trace one event across edge processing and cloud ingestion |
| Docker Compose | Reproduce the complete system locally |
| Terraform | Reviewable and reproducible cloud infrastructure with broad production relevance |

The cloud provider remains deliberately undecided until deployment work starts in
v0.9. The project should not add Kafka, Kubernetes, a managed message broker or
additional microservices unless a measured requirement makes them necessary.

## Demonstration Scenarios

The repository should contain scripted scenarios that expose real engineering
trade-offs rather than only showing the happy path.

### Scenario 1: Offline backlog recovery

```text
Cloud unavailable
→ edge continues processing
→ events accumulate in SQLite
→ edge restarts
→ pending events remain
→ cloud returns
→ backlog is delivered
```

Expected evidence:

* no local event loss;
* no interruption of local control;
* outbox depth visible through metrics;
* eventual delivery of every valid event.

### Scenario 2: Lost acknowledgement

```text
Cloud commits a batch
→ response is deliberately lost
→ edge retries the same events
→ cloud identifies duplicates
```

Expected evidence:

* one durable cloud row per event;
* duplicate counters increase;
* edge eventually marks every event delivered.

### Scenario 3: Mixed-quality batch

```text
Batch contains:
- one valid event
- one duplicate
- one invalid measurement
- one old but historically valid event
```

Expected evidence:

* partial acceptance has explicit semantics;
* invalid data does not corrupt latest state;
* old telemetry may be retained without replacing newer state;
* the response explains every outcome.

### Scenario 4: Database outage

```text
Ingestion service remains alive
→ PostgreSQL becomes unavailable
→ readiness fails
→ requests receive no false acknowledgement
→ PostgreSQL recovers
→ pending edge events are retried successfully
```

### Scenario 5: Slow cloud and backpressure

```text
Telemetry arrives faster than it can be uploaded
→ queues remain bounded
→ local processing continues according to defined limits
→ backlog and latency become visible
```

The system must document what happens when local storage reaches its configured
limit. It must not silently pretend that capacity is unlimited.

## Engineering Decision Records

Important architectural choices should be captured as short ADRs. The set is
kept deliberately small. A record is written only when a decision had at least
two credible alternatives and a real cost, and only after the behaviour it
describes exists.

Recorded, in [`adr/`](adr/):

```text
ADR-001 — Deploy one edge agent per household or site
ADR-002 — Keep control local and independent of cloud infrastructure
ADR-003 — Persist each observation atomically before applying its command
ADR-004 — Classify every observation and never regress latest state
```

Planned, to be written after the corresponding implementation:

```text
ADR-005 — Deliver through a durable outbox with at-least-once semantics (v0.7)
```

Each ADR should contain:

```md
# Decision title

## Context

What concrete problem or constraint required a decision?

## Decision

What was selected?

## Alternatives considered

What realistic alternatives were rejected?

## Consequences

What becomes easier, harder or impossible because of this decision?

## Revisit when

Which measurable condition would justify reconsidering it?
```

The `Revisit when` section is important. It proves that a decision is contextual,
not a permanent claim that one technology is always superior.

## Postmortems

Wattfeder may include postmortems for failures discovered during implementation,
testing or controlled incident exercises.
Do not invent production incidents. Clearly label simulated failures as incident
exercises.

Useful examples:

* duplicate cloud records caused by acknowledgement handling;
* latest device state overwritten by delayed telemetry;
* unbounded retry loop exhausting resources;
* readiness reporting healthy during database failure;
* edge shutdown losing events that were accepted but not persisted;
* database migration preventing service startup.

Each postmortem should include:

```md
# Incident or exercise title

## Summary

What failed?

## Impact

Which data, component or guarantee was affected?

## Timeline

What happened and in which order?

## Root cause

Which design or implementation defect allowed the failure?

## Contributing factors

What made detection or recovery harder?

## Resolution

How was service restored?

## Corrective actions

Which code, tests, metrics, documentation or operational procedures changed?

## Verification

How does the repository now prove that the failure cannot silently recur?
```

A strong postmortem should end with concrete evidence such as a regression test,
metric, alert, invariant or changed transaction boundary.

## Portfolio Completion Criteria

Wattfeder is portfolio-ready when another engineer can:

1. understand the architecture in a few minutes;
2. start the complete local system with one documented command;
3. run the important failure scenarios;
4. trace one event from the simulator through SQLite and into PostgreSQL;
5. inspect the relevant logs and metrics;
6. understand the major trade-offs through ADRs;
7. verify the claimed guarantees through automated tests;
8. deploy the ingestion service through documented infrastructure code.

The project is not complete merely because all planned features exist. It is
complete when its most important reliability claims are demonstrated and easy to
review.