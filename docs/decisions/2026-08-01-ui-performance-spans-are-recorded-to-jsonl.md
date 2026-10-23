# UI performance spans are recorded as raw JSONL, not aggregated metrics

- **Status:** accepted
- **Date:** 2026-08-01

## Context

A slow interaction spans a click handler, a Wails RPC and a render, and pprof
(ADR pprof-debug-endpoint) sees only the Go half. The intended workflow is that
an agent instruments a suspected path, the app is exercised, and the timings
are read back.

Prometheus histograms would pre-aggregate away the outlier being chased, and
the context worth attaching (item id, repo, session id) is exactly what blows up
label cardinality. JSON lines are directly readable by the agent doing the
analysis.

## Decision

1. **The record is a span, and every sample is kept.** `perf.Sample` carries
   scope, name, duration, timestamp, free-form attributes, and optional
   `id`/`parent`, in an OpenTelemetry-compatible shape so the file could be
   converted to OTLP later. Nothing is aggregated at write time.

2. **`cmd/desktop/internal/app/perf` owns `perf.jsonl` with no third-party
   dependency.** It rotates one generation (`perf.jsonl.1`) at `MaxBytes`
   (8 MiB default), so worst-case disk use is twice that and a rotation
   mid-session cannot lose the run being debugged.

3. **The recorder assigns the sequence number**, because the frontend cannot
   keep a monotonic counter across a reload. A batch is written under one lock
   so its samples land together.

4. **An invalid sample is dropped, not raised.** `Record` reports how many
   landed. Ad hoc instrumentation must not break a flush or the UI. Attribute
   count and size are bounded for the same reason.

5. **The gate is `development.perf.enabled`, off by default.** `devtools`
   writes `HIVE_DESKTOP_DEVELOPMENT_PERF_ENABLED=true` into `launch.env`, so a
   dev session records without opting in and a shipped build never opens the
   file. When off, the recorder is a no-op object rather than nil, and the
   frontend stops buffering once `Info` answers.

## Consequences

- Analysis is `jq` or an agent reading the file. There is no query API or
  dashboard.
- Instrumentation can be added and left in freely; spans are inert in a
  shipped build.
- Samples are wall-clock durations from `performance.now()` in the frontend,
  covering RPC and render, so they are not comparable to a Go profile.
- The file never leaves the machine; it is not a telemetry channel.
