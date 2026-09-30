# UI performance spans are recorded as raw JSONL, not aggregated metrics

- **Status:** accepted
- **Date:** 2026-08-01

## Context

Slow UI interactions are the hardest thing in this app to diagnose. pprof
(ADR pprof-debug-endpoint) profiles the Go side, but an interaction that feels slow spans a
click handler, a Wails RPC, and a render — and nothing observes the frontend
half. The intended workflow is that an agent adds instrumentation to a
suspected path, the app is exercised, and the timings are read back.

Prometheus was the obvious alternative and was rejected. A histogram
pre-aggregates, and the outlier is the bug being chased: knowing 1% of renders
exceeded 2s does not identify which render or what it was doing. The context
worth attaching — item id, repo, session id — is exactly what blows up label
cardinality. Metric names arriving from the frontend at runtime also fight a
client library built around statically declared collectors: supporting them
means a lazily-populated registry keyed by name and label set, which is more
bespoke infrastructure than a file append, not less. And a `/metrics` scrape
with no time-series database behind it says almost nothing, while JSON lines
are directly readable by the agent doing the analysis.

## Decision

1. **The record is a span, and every sample is kept.** `perf.Sample` carries
   scope, name, duration, timestamp, free-form attributes, and optional
   `id`/`parent`. The shape is deliberately OpenTelemetry-compatible so the
   file can be converted to OTLP if a trace viewer is ever wanted, without
   re-instrumenting anything. Nothing is aggregated at write time — the
   analysis step decides what to aggregate.

2. **`internal/app/perf` owns the file and has no third-party dependency.**
   `encoding/json` writes one line per record; the size cap and its single
   rotation generation (`perf.jsonl.1`) are ours, so worst-case disk use is
   2 × `MaxBytes` (8 MiB default) per instance. A rotation mid-session
   therefore cannot lose the run being debugged.

3. **The sequence number is assigned by the recorder, not the caller.** The
   frontend cannot produce a monotonic counter that survives a page reload, and
   write order is what makes the file readable. A batch is written under one
   lock so its samples land contiguously.

4. **An invalid sample is dropped, not raised.** `Record` reports how many
   landed and skips the rest. Instrumentation added ad hoc to chase a bug must
   not be able to break a flush — or the UI — with one malformed record.
   Attribute count and value size are bounded for the same reason.

5. **The gate is `development.perf.enabled`, off by default**, following
   pprof's precedent. `dev` turns it on through `launch.env`, so a dev
   session records without anyone opting in and a shipped build never opens a
   file. When off the recorder is a no-op object rather than nil — no call site
   needs a nil check — and the frontend stops buffering after `Info` answers,
   so instrumentation left in the code costs a boolean check.

## Consequences

- Analysis is whatever the reader brings: `jq` over `perf.jsonl`, or an agent
  reading it directly. There is deliberately no query API, no summary endpoint,
  and no dashboard — adding one is cheap later if the raw file proves awkward,
  and the file is the input either way.
- Instrumentation is expected to be added and removed freely as things are
  investigated. Spans left behind are inert in a shipped build, so there is no
  need to strip them before merging.
- The samples are wall-clock durations from `performance.now()`, measured in
  the frontend. They include everything the user waits for — RPC and render —
  which is the point, and means they are not comparable to a Go-side profile.
- `id`/`parent` are recorded but nothing populates them yet. They exist so
  nesting is expressible without a schema change once a span tree is worth
  building.
- This is not a telemetry channel. It never leaves the machine and has no
  relationship to the analytics the future admin server is planned to collect.
