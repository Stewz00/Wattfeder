# ADR-004: Classify every observation and never regress latest state

**Status:** Accepted. Implemented in v0.3. Recorded retrospectively.

## Context

Until v0.2, Wattfeder assumed every telemetry event was unique, valid, and in
order. A real device breaks all three assumptions. Producers resend the same
event. The network delays and reorders deliveries. A measurement can be missing
or impossible. A device can go quiet, or report itself unavailable.

v0.2 stopped processing at the first duplicate. One bad interval then silenced a
device for the rest of the run.

Wattfeder therefore needed three things: a defined outcome for each of those
cases, a durable signal of how trustworthy a device is, and a home for both. The
cloud ingestion service in v0.6 will apply the same rules, so they cannot live
in the SQLite adapter.

## Decision

Every observation of one interval gets exactly one disposition. Processing
continues afterwards, whatever that disposition was.

| Case | Disposition | History | Latest state | Command | Health |
| --- | --- | --- | --- | --- | --- |
| Valid, new, arrived on time | `accepted` | Store | Update | Apply | `online` |
| Valid, new, arrived late | `accepted` | Store | Update | Suppress | `stale` |
| Valid, but not newer than latest state | `history_only` | Store | Keep | Suppress | Re-evaluated, `invalid` stays |
| Event ID already known | `duplicate` | No change | Keep | Suppress | Unchanged |
| Missing value, impossible measurement, wrong device, or future event time | `rejected` | Discard | Keep | Suppress | `invalid` |
| Heartbeat missing | `missing` | No change | Keep | Suppress | `stale`, later `offline` |
| Source reports unavailability | `unavailable` | No change | Keep | Suppress | `offline` |

**Ordering.** Latest state is ordered by UTC event time alone. Receive time and
arrival order never decide it. Event ID is used only to detect duplicates. An
event that claims to happen after it arrived is rejected, so a wrong clock
cannot poison the latest state.

**Lateness.** An event is late when it arrives more than one telemetry interval
after its event time. It still becomes the latest state, because it is still the
newest measurement. Its command is suppressed, because the interval it describes
is already over.

**Old events.** An event that is not newer than the latest state is stored as
history instead of being dropped. History stays complete for audit and replay,
and the latest state still only moves forward in event time. Such an event
proves the device is transmitting, so it updates last contact, but it cannot
clear an `invalid` health status.

**Two timestamps.** Every stored telemetry record keeps both. Event time comes
from the producer and drives ordering. Receive time comes from Wattfeder's own
clock and drives lateness, contact age, and the audit trail.

**Health.** A device is `online`, `stale`, `offline`, or `invalid`, decided in
this order:

1. `offline` — no contact within the offline threshold, or the source reported
   itself unavailable.
2. `invalid` — the last observation was rejected and nothing valid and newer has
   arrived since.
3. `stale` — the newest accepted event is older than the stale threshold.
4. `online` — otherwise.

Only a valid, newer event restores health, and it always does, because its
arrival proves current contact. Reported unavailability forces `offline`
immediately. The thresholds default to two and three telemetry intervals, and
any override must keep `0 < stale < offline`.

**Placement.** Disposition and health are decided by one pure domain function,
`household.Classify`. The storage adapter calls it after looking up whether the
event ID already exists, then applies the result in one transaction. Inside that
transaction the adapter checks again that the event really is newer. A duplicate
changes nothing at all, including health.

## Alternatives considered

* **Reject events that are not newer instead of storing them.** Rejected. It
  throws away a legitimate audit trail, and a merely late measurement becomes
  indistinguishable from bad data.
* **Order the latest state by receive time.** Rejected. Reordered delivery would
  then let an older measurement overwrite a newer one.
* **Count consecutive missed heartbeats instead of measuring contact age.**
  Rejected. A counter resets on any contact, however stale that contact's data
  is, and it cannot share thresholds with the staleness rule commands already
  need.
* **Stop processing at the first anomaly, as v0.2 did.** Rejected. One bad
  interval would silence a device.
* **Put the rules in the storage adapter.** Rejected. The cloud ingestion
  service needs the same rules, and it will not use SQLite.

## Consequences

Replaying a stored day is safe and readable. Every interval reports its
disposition, and a duplicate, a missed heartbeat, or a single impossible
measurement no longer stops the intervals after it. Health gives an operator one
durable answer about a device, independent of any single interval.

The costs are real. The schema carries disposition, reason, and health columns.
The application loop writes a record for every interval, even when there is no
telemetry and no command. The adapter re-checks a rule its caller already
applied.

Suppressing commands on late and historical events means a site can run several
intervals without a fresh control decision on a slow link. That is the safe
behaviour, but it is not the obvious one.

## Revisit when

Any of three things happens. Several writers share one device's records; the
re-check inside the transaction assumes a single SQLite writer. Command timing
needs finer resolution than one interval, so "late" needs a sharper boundary. A
command must be delivered exactly once; today a suppressed one is skipped, not
queued.
