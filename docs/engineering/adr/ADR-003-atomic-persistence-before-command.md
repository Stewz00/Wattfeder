# ADR-003: Persist each observation atomically before applying its command

**Status:** Accepted. Implemented in v0.2 and extended in v0.3. Recorded
retrospectively.

## Context

Processing one interval produces up to three durable facts: the telemetry
event, the resulting latest device state, and the command. Writing them
separately allows a crash to leave a command without its telemetry, or state
that no stored event explains. Retried telemetry also has to be recognisable,
otherwise the same interval is processed twice.

## Decision

The telemetry producer assigns a stable event ID before validation. A retry of
the same source event keeps that ID.

Telemetry, latest state, command and device health commit in one transaction.
Each part is written only when the observation's disposition calls for it, and
device health is always written. A commit with an already-known event ID
changes nothing at all and reports a duplicate.

The commit completes before the command is applied. If persistence fails,
nothing was written and the battery state does not advance.

## Alternatives considered

* **Apply the command first and persist afterwards.** Rejected. A crash would
  leave a battery acting on an event the system cannot prove it received.
* **Separate writes per record.** Rejected. It creates partial states that
  every later reader would have to repair.
* **Deduplicate on device ID and timestamp.** Rejected. A later telemetry
  source may legitimately send several events for one device and timestamp.

## Consequences

A crash cannot produce a command without its telemetry, and duplicate delivery
cannot double-apply an interval. Every durable record is traceable to one event
ID, which is also what the cloud ingestion service will use for idempotency.

One failure window remains, and it is not closed by this decision: a crash
after the commit but before the command is applied leaves durable state ahead
of command delivery. Wattfeder currently has no command sink to acknowledge
delivery, so this window is documented rather than handled.

## Revisit when

A real command sink exists. Delivery state, acknowledgements, idempotent
command IDs, or reconciliation after restart will then be needed to close the
remaining window.
