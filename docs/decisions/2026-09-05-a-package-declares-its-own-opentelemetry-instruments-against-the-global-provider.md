# A package declares its own OpenTelemetry instruments against the global provider

- **Status:** accepted
- **Date:** 2026-09-05

## Context

A package that emits telemetry needs a way to reach the providers. Hiding
OpenTelemetry behind a consumer-defined interface per package fixes the
attribute set at the interface's signature, forces allocations the API
avoids, and has to re-expose every capability the API grows
([Don't Wrap OpenTelemetry](https://opentelemetry.io/blog/2026/dont-wrap-opentelemetry/)).
Tests do not need the wrapper either: the SDK ships in-memory readers.

## Decision

1. **`telemetry.New` registers the providers globally.** The MeterProvider
   is registered when either the export or the scrape gate is on. The
   TracerProvider is registered only for export, because spans have no
   local sink. Registration comes after every exporter is built, so a
   construction failure never leaves a shut-down provider reachable through
   the global.

2. **The boundary is the API/SDK split.** Any package may import
   `go.opentelemetry.io/otel`, `/trace` and `/metric` and declare
   package-level instruments. Only `cmd/desktop/internal/app/telemetry` may
   import `/sdk/...` and the exporters, which hold nearly all of the binary
   cost. depguard enforces it (`otelsdk` in `.golangci.yml`); test files are
   exempt so they can assert with the SDK's manual reader.

3. **`internal/platform/observe` holds the scope-name convention and a few
   helpers, not an abstraction.** `Tracer` and `Meter` prepend the module
   path and return the real API types. `Must` unwraps an instrument
   constructor, `RecordError` also sets the span status, and
   `StartConditionalSpan` opens a span only under an existing one.

4. **A metric attribute must have a bounded domain.** Per-user values such
   as a session or window name are unbounded and cost a series each for as
   long as it lives. `tmux.stream.lifecycle` carries only `state`
   (`paused`/`resumed`); which session was noisy is a log or span question.

5. **Instrumentation a library already provides is adopted, not written.**
   `sourcehttp` wraps its transport in `otelhttp.NewTransport`, so the HTTP
   metrics are the semconv ones dashboards already know.

## Consequences

- The global providers are process-wide, so a test that asserts on an
  unlabelled instrument cannot run in parallel with another that moves the
  same series.
- A measurement takes a real context (for trace correlation), never
  `context.Background()` inside a core method.
- What deserves a span is ADR a-span-is-a-trigger-or-a-wait-and-its-count-per-trigger-is-bounded-by-configuration.
