# Operations

This runbook covers running and diagnosing one edge agent. It assumes the
agent was started with `-ops-address` set, so `/healthz`, `/readyz`, and
`/metrics` are available; see [SETUP.md](SETUP.md) for flags.

## Health vs. readiness

`/healthz` is liveness: the process is alive and serving HTTP. It returns 200
whenever the process runs at all. If it fails, the only fix is a restart.

`/readyz` is readiness: this agent can currently process and persist
telemetry. It answers a different question than health, and it is expected to
flap — turning red while the agent starts up, or while storage is failing, is
correct behavior, not a bug. It returns 503 with a JSON body naming the
failing check:

```json
{"status": "not ready", "failing_check": "telemetry"}
```

| Check | Ready when | Not ready when |
| --- | --- | --- |
| `telemetry` | An interval completed within 3x the configured `-interval`. | No interval has completed yet (the agent is starting), or the processing loop has stalled. |
| `storage` | The most recent interval ended without an error. | The most recent interval ended in an error — most often a failed commit. The run itself is ending in this case, so readiness turning red is the signal to stop sending traffic before the process exits. |

Device health (`online` / `stale` / `offline` / `invalid`) never affects
readiness. That describes the household this agent manages, not the agent
process itself — see the `wattfeder_device_health` gauge below.

## Metrics

Served at `/metrics` in Prometheus text format, on a private registry (never
the global default, so nothing outside this agent can register a colliding
series):

| Metric | Type | Meaning |
| --- | --- | --- |
| `wattfeder_telemetry_received_total` | counter | An envelope arrived. Does not increment for a missing heartbeat, since no envelope arrived at all. |
| `wattfeder_telemetry_processed_total{disposition}` | counter | One per interval, labeled by disposition (`accepted`, `history_only`, `duplicate`, `rejected`, `missing`, `unavailable`). |
| `wattfeder_commands_created_total{decision}` | counter | One per command actually created (`charge`, `discharge`, or the idle equivalent). |
| `wattfeder_device_health{status}` | gauge | 1 on the currently active health status, 0 on the other three. |
| `wattfeder_processing_duration_seconds` | histogram | Time for one interval: source, classify, commit, apply, write. |
| `wattfeder_event_lag_seconds` | gauge | Receive time minus event time for the most recently timestamped telemetry. Untouched by an interval that carried none (a missing heartbeat, for instance), so it always reflects the last real measurement rather than resetting to zero. |

Two metrics that might be expected are deliberately absent:

- `wattfeder_queue_depth` — no queue exists at the edge. The v0.7 outbox will
  introduce one; this metric arrives with it.
- `wattfeder_telemetry_rejected_total` — this is one label of
  `wattfeder_telemetry_processed_total{disposition="rejected"}`, not a
  separate series.

## Logs

Structured JSON, one line per interval, written to **stderr** — stdout carries
only the record stream, so logs and records can always be split with
`1>records.jsonl 2>agent.log`. Each interval line carries `agent_id`,
`device_id`, `event_id`, `disposition`, `disposition_reason`, `health_status`,
`decision`, `duration_ms`, `event_lag_seconds`, and, when tracing is enabled,
`trace_id` — the same ID as the span for that interval, so a log line and its
trace can be cross-referenced directly.

Rejected and unavailable observations log at `warn`; accepted, history-only,
and duplicate observations log at `info`; a run-ending failure logs at
`error`. `-log-level` controls the minimum level written.

## Tracing

Set `-otlp-endpoint` (for example `localhost:4318`) to export traces over
OTLP/HTTP. Each interval produces one span named `interval`, with a child span
`commit_processing` around the durable commit — enough to tell "the telemetry
source was slow" from "storage was slow" by looking at which span in the trace
took the time. With no `-otlp-endpoint`, no exporter starts and there is no
tracing overhead.

## Common failure scenarios

**Telemetry stopped arriving.** `/readyz` turns red with `failing_check:
telemetry`. Check the log for the most recent `interval_processed` line's
timestamp; if it is older than 3x the configured `-interval`, the telemetry
source itself has stopped producing observations — this is upstream of the
agent.

**A device is stale or offline.** This is visible in
`wattfeder_device_health` and in each record's `health_status`, but it does
not affect `/readyz`: an agent whose household hasn't reported recently is
still a healthy, ready agent. Compare `health_transition_at` in the record
stream against the current time to see how long the device has been in that
state.

**Readiness is red.** Read the `failing_check` field. `telemetry` means no
interval has completed recently (see above). `storage` means the most recent
interval ended in an error, almost always a failed commit; the process is
ending on its own in this case; check the log's `error` field on the last
`interval_processed` line and the process's exit status.

**Processing is slow.** `wattfeder_processing_duration_seconds` covers the
whole interval; the trace for a slow interval shows whether the time went into
the `commit_processing` child span (storage) or the rest of the parent span
(everything else — telemetry source and classification). `duration_ms` on the
log line gives the same number without needing a trace backend running.

**The database is unwritable.** `CommitProcessing` returns an error, the
interval ends in error, `/readyz` turns red with `failing_check: storage`, and
the run itself returns that error and exits non-zero. Nothing silently
continues with a broken database.

**What shutdown does.** SIGINT or SIGTERM cancels the run's context.
Cancellation is checked once per interval, before the next observation is
requested, so an observation still in flight when the signal arrives is never
abandoned mid-processing: it finishes committing and applying its command on a
context bounded by `-shutdown-grace`, then the run exits. `/healthz` and
`/readyz` stop serving once the ops server shuts down, and the tracer
provider, if any, flushes pending spans before shutdown completes.

**What state is lost when the process stops.** None, for any interval that
was fully classified before cancellation arrived — its telemetry, latest
state, command, and health are already durable by the time the process exits.
Only an interval whose observation had not yet been pulled from the source
when cancellation arrived is skipped; it was never accepted, so nothing is
lost that the agent had taken responsibility for.
