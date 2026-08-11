# ADR-002: Keep control local and independent of cloud infrastructure

**Status:** Accepted. Recorded in v0.3. The domain contains no transport or
storage code today. An automated import check enforces it from v0.4, and the
cloud service that tests the claim arrives in v0.6.

## Context

Wattfeder will get a cloud ingestion service. The tempting design is to send
telemetry up and let the cloud answer with a command. A household battery,
however, must keep working while the internet does not. The project therefore
had to decide whether the cloud is part of the control loop or not.

## Decision

Battery control runs locally and keeps running when the cloud is unavailable.

Domain policy does not depend on transport or persistence. Telemetry reaches
the domain through an adapter. Durable storage and delivery sit behind
interfaces the domain owns. Cloud ingestion observes what already happened; it
never decides it.

## Alternatives considered

* **Cloud-decided commands.** Rejected. A connectivity failure would stop
  control of a physical battery.
* **Cloud in the loop with a local fallback policy.** Rejected. Two policies
  would have to agree, and the fallback would be the least tested code in the
  system while being the code that runs during an incident.

## Consequences

Connectivity failure cannot stop control. Cloud ingestion stays outside the
control loop, so it can be slow, batched, or offline without harm. Edge state
and cloud state may disagree for a while, and the cloud is eventually
consistent by design.

The cost is real. Configuration and policy updates become harder, because they
must reach each agent. Fleet-wide optimisation cannot make every immediate
decision. Any future cloud-side optimisation has to work through configuration
or setpoints rather than direct commands.

## Revisit when

Control needs global fleet information, market coordination, or cloud-scale
forecasting that an isolated agent cannot compute.
