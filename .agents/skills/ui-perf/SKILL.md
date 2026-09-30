---
name: ui-perf
description: Measure a slow UI interaction by instrumenting the frontend with usePerf and analyzing the spans it writes to perf.jsonl. Use when asked why something in the app feels slow, to time a click/render/fetch path, to find which interaction is costing the most, or to confirm a performance fix actually moved the number.
compatibility: Requires a desktop instance from this worktree with development.perf.enabled on (the default in every prepared worktree via launch.env). jq.
---

# Measure a slow UI interaction

`usePerf` records **spans** — one completed operation with a duration, a name,
and free-form attributes — to a JSONL file the Go side owns (ADR ui-performance-spans-are-recorded-to-jsonl). Every
sample is kept; nothing is aggregated at write time, so the outlier that is
usually the actual bug survives to the analysis step.

Pick the right tool first:

- **A slow *interaction*** — a click that takes a beat, a view that stutters on
  open, a fetch whose cost is unclear — is this skill. It measures what the user
  actually waits for, RPC and render included.
- **A hot *Go* path** — CPU in the producer, the flow engine, a store query — is
  pprof (ADR pprof-debug-endpoint), enabled with `development.pprof.enabled` and mounted at
  `/debug/pprof/` on the loopback server. These numbers are not comparable;
  a span includes time pprof never sees.

## 1. Confirm recording is on

The gate is `development.perf.enabled`, off in a shipped build and set for
`dev` through `launch.env`:

```bash
grep HIVE_DESKTOP_DEVELOPMENT_PERF_ENABLED launch.env   # expect "true"
```

If it is missing, the worktree's `launch.env` predates the setting — run
`mise run desktop:dev:prepare`, which regenerates it. On startup the log carries
`UI performance recording enabled` with the file path.

From the frontend, `perfInfo()` answers the same question at runtime and returns
the path. When recording is off, everything below is inert: the composable stops
buffering after the first `Info` call and no file is opened.

## 2. Instrument the suspected path

Import in the component or composable that owns the interaction:

```ts
import { usePerf } from '../composables/usePerf'   // relative — this project has no @ alias

const perf = usePerf('feed')   // scope = subsystem: feed, terminal, pipeline
```

Three shapes, in order of preference:

```ts
// Wrap work, sync or async. Result passes through; a throw is still recorded
// with failed: true, then rethrown.
const items = await perf.track('items:load', () => loadItems(id), { id })

// Open and close a span when the work is not one call — a render that finishes
// on nextTick, a handler that spans an await you do not own.
const end = perf.start('view:open', { itemId })
await nextTick()
end({ count: items.length })          // attrs from both ends are merged

// Report a duration you measured some other way (a PerformanceObserver entry,
// a timestamp delta from the backend).
perf.record('first-paint', entry.duration)
```

**Name stability is what makes the data usable.** `name` identifies the
operation and must be the same string on every call so samples group; the part
that varies goes in attrs. `track('item:open', fn, {id})` is right;
interpolating the id into the name gives every sample a unique one and nothing
aggregates.

Attributes are the reason raw spans beat a histogram — put the context you would
want when reading an outlier in them (`itemId`, `repo`, `count`, `cached`).
Bounds: 32 attributes, 512 bytes per key or string value. An over-budget or
malformed sample is dropped silently rather than breaking the flush, so if a
span never appears, check it against `Sample.Validate` in
`cmd/desktop/internal/app/perf/perf.go`.

## 3. Exercise the app and collect

Samples buffer in the frontend and flush every 2s (or at 256 buffered, or on
`pagehide`), so the file lags the interaction slightly. Drive the app —
`mise run desktop:dev` for a Vite HMR loop, or `mise run desktop:serve` plus browser tooling
for a headless one — then find the file:

```bash
PERF="$(grep HIVE_DESKTOP_DATA_DIR launch.env | cut -d'"' -f2)/desktop/perf.jsonl"
wc -l "$PERF"
```

Truncate between runs so an experiment is not read against the last one's
samples — the app reopens the file in append mode and does not mind:

```bash
: > "$PERF"
```

## 4. Analyze

One line per span: `{seq, at, scope, name, durationMs, attrs}`. `seq` is
write order and is assigned by the recorder, so it orders the file even across
a page reload.

**Where the time is going** — count and percentiles per operation, worst first:

```bash
jq -s '
  group_by(.scope + " " + .name)
  | map({
      op: (.[0].scope + " " + .[0].name),
      n: length,
      p50: (sort_by(.durationMs) | .[(length * 0.5 | floor)].durationMs),
      p95: (sort_by(.durationMs) | .[(length * 0.95 | floor)].durationMs),
      max: (max_by(.durationMs).durationMs)
    })
  | sort_by(-.p95)
' "$PERF"
```

**The individual slow ones**, with their context — usually the fastest route to
a cause, and the thing an aggregate would have destroyed:

```bash
jq -s -c 'sort_by(-.durationMs) | .[:10] | .[] | {seq, name, durationMs, attrs}' "$PERF"
```

**Is one input the problem?** Group an operation by an attribute:

```bash
jq -s -c '[.[] | select(.name == "items:load")]
  | group_by(.attrs.repo)
  | map({repo: .[0].attrs.repo, n: length, avg: ((map(.durationMs) | add / length) * 100 | round / 100)})
  | sort_by(-.avg)' "$PERF"
```

**Spans that ended in an error** — a slow path that fails is worth seeing
separately:

```bash
jq -c 'select(.attrs.failed == true) | {seq, name, durationMs, attrs}' "$PERF"
```

Read `seq` ranges to reconstruct a sequence: spans land in completion order, so
a slow parent surrounded by its children is visible as a `seq` neighbourhood.

## 5. Report and clean up

Quote the numbers that moved and the attrs that explain them, not the whole
file. When a fix lands, re-run the same interaction against a truncated file and
compare p95 for the same operation name.

**Instrumentation can stay.** It is inert in a shipped build, so a span left on
a path that turned out to be interesting does not need stripping before merge.
Remove what was pure noise; keep what would help the next investigation.

## Guardrails

- **Do not add aggregation, a summary endpoint, or a metrics exporter.** The raw
  file is the interface by decision (ADR ui-performance-spans-are-recorded-to-jsonl) — the reasoning against
  pre-aggregated metrics is in that ADR, so read it before proposing one.
- **Do not interpolate varying values into `name`.** It is the single most
  common way to make a run unanalyzable.
- **Do not instrument inside a tight render loop** without checking the cost.
  Each span is an object plus a buffer push; thousands per second will rotate
  the file (8 MB cap, one previous generation kept as `perf.jsonl.1`) and push
  the interesting samples out.
- **The file never leaves the machine.** It is not telemetry and has no
  relationship to any analytics the app may later collect. It may contain repo
  names, item ids, and paths — treat it as local diagnostic data and do not
  paste it wholesale into an issue.
