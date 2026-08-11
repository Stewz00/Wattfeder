# ADR-001: Deploy one edge agent per household or site

**Status:** Accepted. Recorded in v0.3. One process already handles exactly one
household; the multi-instance deployment it describes arrives in v0.4.

## Context

A household energy system produces telemetry and needs battery commands. It was
not obvious where that work belongs. One gateway could serve several
households. The cloud could process every household centrally. Wattfeder had to
pick one model, because the choice decides who owns the local database, what a
device ID means, and where concurrency lives.

## Decision

One edge-agent instance manages one household or energy site.

Each agent owns its own SQLite database. Each agent processes only its own
telemetry. `device_id` names the one logical household energy system that agent
manages. Fleet-wide concurrency, partitioning and load management are cloud-side
concerns.

## Alternatives considered

* **One gateway serving several households.** Rejected. It forces multi-tenant
  writes into a single-writer SQLite database and turns a local fault into a
  fault for every household behind that gateway.
* **Cloud-managed household processing.** Rejected. Control would stop whenever
  the connection stopped. See [ADR-002](ADR-002-local-control-independent-of-cloud.md).

## Consequences

SQLite contention disappears, because there is one writer per database. A crash
affects one household only. Concurrency belongs to the cloud ingestion service,
not to the edge agent, so the edge code stays simple. Fleet load testing must
emulate many independent agents against the cloud service instead of starting
many local processes.

The cost is more running instances, and per-instance configuration and identity
that must stay distinct.

## Revisit when

One physical gateway must serve several independent sites for economic or
operational reasons.
