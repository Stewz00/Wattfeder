# ADR-008: Disposition, Ordering, and Health Semantics for Unreliable Telemetry

## Context

Through v0.2, Wattfeder assumed every telemetry event was unique, valid, and
delivered in order. That assumption does not hold for a distributed or
intermittently connected device: producers retry and resend the same event,
network paths reorder or delay delivery, measurements can be missing or
invalid, and a device can stop sending heartbeats or explicitly report itself
unavailable.

v0.3's scope (see `docs/roadmap.md`) requires defined behavior for duplicate,
out-of-order, delayed, incomplete, and invalid events, missing heartbeats, and
explicit unavailability, plus a durable device health signal derived from
that behavior. The roadmap also requires an explicit split between when an
event happened (event time) and when Wattfeder received it (receive time),
and a rule that old telemetry may be stored as history but must never replace
a newer latest state. Because the same disposition matrix is reused by the
cloud ingestion service in v0.6, its semantics cannot depend on where
processing happens, and it must live in domain code rather than in a specific
storage adapter.

This ADR consolidates the two related decisions the roadmap calls out for
v0.3 — the disposition matrix with its ordering key and event/receive-time
split, and storing old telemetry without allowing stale latest-state updates
(GOALS.md's ADR-008 slot) — because they are one coherent decision about how
one interval's observation is classified and durably recorded.

## Decision

### Disposition matrix

Every observation for one interval receives exactly one disposition:

| Case | Disposition | History | Latest state | Command | Health |
| --- | --- | --- | --- | --- | --- |
| Unique, valid, fresh, strictly newer event | `accepted` | Store | Update | Apply | `online` |
| Unique, valid, delayed, strictly newer event | `accepted` | Store | Update | Suppress | `stale` |
| Unique, valid event with event time equal to or older than latest | `history_only` | Store | Preserve | Suppress | Re-evaluate without clearing `invalid` |
| Existing event ID | `duplicate` | No change | Preserve | Suppress | Preserve |
| Missing value, invalid measurement, device mismatch, or future event time | `rejected` | Do not store | Preserve | Suppress | `invalid` |
| Missing heartbeat | `missing` | No change | Preserve | Suppress | Become `stale`, then `offline` at configured boundaries |
| Explicit source unavailability | `unavailable` | No change | Preserve | Suppress | Immediately `offline` |

A duplicate commit changes no durable record at all, including health — it is
a complete no-op discovered by the storage layer's own uniqueness check, not
predicted in advance by the classifier.

### Ordering key

Latest state is ordered strictly by UTC event time. Receive time and arrival
sequence never select latest state: an event with an event time equal to or
older than the current latest state is stored as history only. Event ID is
used solely for deduplication, never for ordering.

"Delayed" is judged independently of ordering: an otherwise-accepted event is
delayed when its receive time is more than one telemetry interval after its
event time. A delayed event still becomes the latest state (it is still the
newest event ever seen), but its command is suppressed, because commanding a
battery from a measurement that arrived after the interval it describes had
already elapsed would act on stale information. A delayed event also cannot
be distinguished from a genuinely fresh one by disposition alone; the
suppression is carried on the result, not on the disposition value.

An event time strictly after its own receive time is rejected outright — a
future timestamp must not be allowed to poison the latest projection.

### Event time and receive time

Event time and receive time are both retained on every stored telemetry
record. Event time is producer-asserted and drives ordering; receive time is
Wattfeder's own clock and drives delay detection, health contact-age, and
audit trail. Keeping them separate lets a later delivery-delay or replay
metric be computed without redefining event identity.

### Historical storage rule

An event whose event time is equal to or older than the current latest state
is still stored as history (not discarded), but never updates the latest
state or produces a command. This preserves a complete, gap-free telemetry
history for audit and later reprocessing while keeping the latest-state
projection monotonic in event time. History-only events still update
`last_contact_at` — they prove the source is transmitting — but per the
device-health precedence below they cannot clear an already-`invalid` health
status; only a strictly newer valid event can.

### Health semantics

Device health has four states: `online`, `stale`, `offline`, `invalid`.
Precedence, evaluated in this order:

1. **`offline`** — contact has timed out (no contact within the configured
   offline threshold) or the source explicitly reported unavailability.
2. **`invalid`** — the most recent observation was rejected and no strictly
   newer valid event has arrived since.
3. **`stale`** — the latest accepted event is older than the configured stale
   threshold.
4. **`online`** — otherwise.

Only a strictly newer valid (accepted) event recovers health from `invalid`
or `stale`/`offline`-by-age; recovery is unconditional once that event
arrives, because arrival itself proves current, valid contact. Explicit
unavailability forces `offline` immediately regardless of how recently
contact occurred. Stale and offline thresholds default to two and three
telemetry intervals respectively, and any explicit override must satisfy
`0 < stale < offline`.

### Where this lives

Disposition and health classification are one pure function in domain code
(`household.Classify`) that a storage adapter calls after resolving whether
an event ID already exists. The adapter is still responsible for atomically
applying the result and for defensively re-checking the strictly-newer
condition inside its own transaction — a second line of defense against
races or replay, independent of the caller's classification.

## Alternatives considered

* **Reject non-strictly-increasing events instead of storing them as
  history.** Rejected: it discards a legitimate audit trail and makes a
  correctly-ordered-but-superseded event indistinguishable from actually
  invalid data in later analysis.
* **Order latest state by receive time instead of event time.** Rejected:
  network jitter or reordering would then let a chronologically older
  real-world measurement overwrite a newer one just because it happened to
  arrive first.
* **Track health with a consecutive-miss counter instead of contact age.**
  Rejected: a counter resets on any single contact regardless of how stale
  that contact's data is, and does not share thresholds with the
  already-needed staleness calculation for commands.
* **Stop processing on the first anomaly (the v0.2 behavior for duplicates).**
  Rejected: one bad interval would silence monitoring for an entire device;
  the roadmap's exit criteria require that a rejected or ignored event never
  stops processing of the events that follow it.
* **Have the storage adapter own disposition and health rules directly.**
  Rejected: the same matrix must be reused unchanged by the future cloud
  ingestion service (v0.6), so it belongs in domain code, not in SQLite- or
  service-specific logic.

## Consequences

Replaying a persisted day is now safe and observable: every interval is
reported with its disposition, including duplicates, instead of the run
silently stopping at the first repeat. A single bad measurement, a missed
heartbeat, or a temporarily unavailable source no longer halts processing of
the events that follow it. Health state gives an operator a durable signal of
device trustworthiness independent of any single interval's outcome.

The cost is a wider persisted schema (disposition, disposition reason, and
device health columns) and a substantially more complex application loop:
every interval now produces a structured record even when there is no
telemetry or command to report, and the storage adapter must defend a rule
the caller is also expected to uphold. Command suppression on delayed or
historical events means an edge site can go for multiple intervals without a
fresh control decision during a slow or unreliable link, which is the correct
safety behavior but changes operational expectations.

## Revisit when

* Multiple concurrent writers to the same device's records are introduced;
  the current defensive re-check assumes a single-writer SQLite connection.
* Sub-interval-precision ordering or command timing is required, at which
  point "delayed" needs a more granular boundary than one full interval.
* The cloud ingestion service (v0.6) needs a disposition this matrix does not
  cover, such as cross-device conflict resolution.
* Exactly-once delivery of a command to a physical battery is required; today
  a delayed or suppressed command is simply skipped for that interval, not
  queued or retried.
