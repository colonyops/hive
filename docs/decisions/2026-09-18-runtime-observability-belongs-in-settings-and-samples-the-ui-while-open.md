# Runtime observability belongs in Settings and samples the UI while open

- **Status:** accepted
- **Date:** 2026-09-18

## Context

The runtime dashboard was part of developer tools and therefore hidden behind
`development.devtools.enabled`. The process sampler also registers the gauges
exported through OpenTelemetry for every run, so the data was already a product
observability concern even when its only UI was developer-only.

Telemetry export is configured through `settings.yaml`, but the app did not
show whether OTLP or continuous profiles were enabled and complete, whether
their exporters started, or whether an edit needed a restart. The
developer-tools pane mixed runtime observability with notification tests and a
Wails boundary benchmark.

The UI frame sampler has a different cost from process sampling. Its
`requestAnimationFrame` loop keeps the webview active and prevents it from
idling while the app is otherwise still.

## Decision

1. **Settings ▸ Observability is the user surface.** It shows the runtime
   dashboard and reports enabled, configured, startup, and restart-required
   state for Grafana Cloud OTLP and Profiles destinations. "Exporting" means
   the SDK pipeline started; it does not claim that the remote backend accepted
   data.

2. **`app.ObservabilityService` owns process sampling and export settings.** It
   registers the existing process gauges for the process lifetime and combines
   the effective settings with the startup result passed from the composition
   root. The Wails adapter only converts its types.

3. **Exporter changes apply after restart.** Telemetry participates in logger
   construction before `app.App` exists, so it stays constructed in
   `desktop/main.go`. The core receives a startup status snapshot rather than
   the provider itself.

4. **Configuration remains file-owned.** The page exposes no destination or
   credential fields. It links to the product guide for editing `settings.yaml`,
   where credentials remain `env:`, `file:`, or `op://` references.

5. **UI frame sampling runs only while Observability is open.** Process
   sampling is pull-based and stops with the page's poll. Scoping the animation
   loop to the page gives up historical jank from other views so the shipped app
   can idle when nobody is reading the dashboard.

This supersedes decisions 4 and 5 of ADR
[developer-tools-are-reachable-in-a-shipped-build-behind-a-setting](2026-08-10-developer-tools-are-reachable-in-a-shipped-build-behind-a-setting.md).
The developer-tools gate, route, strip, and Wails round-trip benchmark remain as
specified there.

## Consequences

- Every user can inspect CPU, resident memory, process-tree cost, Go runtime
  state, and current UI frame behavior without enabling developer tools.
- OTLP and Profiles remain independent gates, but the current provider startup
  is all-or-nothing. A failure in either leaves both inactive and the page shows
  the startup error.
- Delivery failures after startup remain in logs and backend diagnostics. The
  UI does not present a connection-health claim the SDK cannot support.
- `perf.jsonl` receives long-frame and event-loop samples only while the runtime
  page is open. The page is a live instrument, not a background profiler.
