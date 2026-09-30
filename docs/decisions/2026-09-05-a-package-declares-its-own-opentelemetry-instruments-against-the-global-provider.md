# A package declares its own OpenTelemetry instruments against the global provider

- **Status:** accepted
- **Date:** 2026-09-05

## Context

ADR telemetry-is-exported-over-otlp-with-no-collector-and-the-same-instruments-serve-a-local-scrape brought in the SDK and left one rule behind:
`internal/app/telemetry` was the only package allowed to import OpenTelemetry,
so a package that wanted to emit had to declare a narrow interface of its own
and take an implementation as a constructor parameter. `tmuxcc.MetricsSink` was
the worked example and `architecture.md` generalised it into the pattern for
every future metric, span and log field.

That is the anti-pattern OpenTelemetry itself documents in
[Don't Wrap OpenTelemetry](https://opentelemetry.io/blog/2026/dont-wrap-opentelemetry/). A wrapper interface fixes
the attribute set at its signature -- `MetricsSink` hardcoded `(session,
window)` and could express nothing else -- forces an allocation where the API
offers allocation-free paths, and has to re-expose every capability the API
grows. Testability was the usual justification and it does not hold either: the
SDK ships in-memory readers for exactly that.

The rule existed because of a real omission rather than a real constraint.
`telemetry.New` built the providers and never registered them, so nothing could
reach them except by being handed something. The rule was the consequence of
that, written down as if it were a principle.

## Decision

1. **`telemetry.New` registers the providers globally**, and that is what makes
   the rest of this possible. The MeterProvider is registered whichever gate is
   on, because the scrape gate alone is enough for an instrument to be worth
   collecting. The TracerProvider is registered only when export is on: spans
   have no local sink, so a provider no reader drains would accumulate for
   nothing. Registration happens after every exporter is built, so a
   construction failure never leaves a provider `fail()` has already shut down
   reachable through the global.

2. **The boundary is the API/SDK split the specification already draws**, not
   "one package may import OpenTelemetry". Any package may import
   `go.opentelemetry.io/otel`, `/trace` and `/metric` and declare package-level
   instruments. Only `internal/app/telemetry` may import `/sdk/...` and the
   exporters. That keeps what the old rule was protecting -- the 4.52 MiB is
   all SDK and exporters, the API packages are small -- and `depguard` enforces
   it, so it is a gate rather than a convention. Test files are exempt: a test
   asserts with `sdkmetric.NewManualReader`, which is the alternative to the
   fake the old rule forced.

3. **`internal/app/observe` holds the scope-name convention and nothing else.**
   `Tracer` and `Meter` prepend the module path and return the real
   `trace.Tracer` and `metric.Meter`, so no call site loses an option the API
   offers. `Must` unwraps an instrument constructor, `RecordError` sets the
   status a bare `RecordError` leaves unset, and `StartConditionalSpan` opens a
   span only when the context already carries one. This is deliberately not an
   abstraction layer -- it holds no type of its own.

4. **A metric attribute must have a bounded domain.** `MetricsSink` took
   `session` and `window`; both are per-user and unbounded, and a metric
   dimension is where that cost is paid on every export for as long as the
   series lives. `tmux.stream.lifecycle` therefore carries `state`
   (paused/resumed) and nothing else, and which session was noisy is a log or a
   span question. The same reasoning as the resource attributes in ADR telemetry-is-exported-over-otlp-with-no-collector-and-the-same-instruments-serve-a-local-scrape,
   applied one level down.

5. **Instrumentation that a library already provides is adopted, not written.**
   `sourcehttp` wraps its logging transport in `otelhttp.NewTransport` rather
   than counting requests by hand: the semconv metrics are the ones dashboards
   already know. otelhttp was already an indirect requirement, so `go.mod`
   gains no dependency tree -- it is promoted to direct, and `httpsnoop` is the
   one transitive module that newly enters the build. `store` ports the sqlc
   `DBTX` decorator from recipinned, naming each span after the `-- name:`
   header sqlc emits.

## Consequences

- `tmuxcc.MetricsSink` and `NopMetrics` are deleted. `TerminalsService` no
  longer takes a sink, and `ObserveFrameLatency` takes a context instead of the
  slug and window it can no longer label with.
- The manager test that used the metrics sink as a synchronisation seam needed
  a real one. `Options.onEmit` is that seam, unexported and nil in production,
  beside the `newProcess` and `loadBuffer` seams the package already had. A
  test that needs to park inside the attach sequence's synchronous first paint
  has no other way in.
- The tmuxcc measurements take `c.lifeCtx`, the client's own lifetime, which is
  what they measure. A metric record is not cancellable work, so the context
  carries trace correlation and nothing else -- but it must be a real context,
  because `context.Background()` inside a core method is forbidden here for
  reasons that stand.
- `store` now opens a span per statement, conditional on an ambient span. Until
  the poll loop is instrumented almost nothing has one, so the practical effect
  today is close to zero and arrives with the trace that gives it a parent.
- `otelhttp` spans on a source client are root spans until the ingest producer
  has a span of its own. That is span volume without a story, and it is the
  argument for instrumenting the producer next rather than last.
- The global providers are process-wide, so a test that asserts on an
  unlabelled instrument cannot run in parallel with another that moves the same
  series. `tmuxcc`'s two instrument tests are sequential and say so.
