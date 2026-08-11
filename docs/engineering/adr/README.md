# Architecture Decision Records

Each record states one decision, the alternatives that were rejected, what the
decision costs, and the condition that would justify changing it.

| Record | Decision |
| --- | --- |
| [ADR-001](ADR-001-one-edge-agent-per-site.md) | Deploy one edge agent per household or site |
| [ADR-002](ADR-002-local-control-independent-of-cloud.md) | Keep control local and independent of cloud infrastructure |
| [ADR-003](ADR-003-atomic-persistence-before-command.md) | Persist each observation atomically before applying its command |
| [ADR-004](ADR-004-observation-disposition-and-device-health.md) | Classify every observation and never regress latest state |

ADR-005 is reserved for delivery through a durable outbox with at-least-once
semantics (v0.7). It will be written only after that behaviour exists.
