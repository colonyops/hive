# Runtime observability belongs in Settings and samples the UI while open

- **Status:** accepted
- **Date:** 2026-09-18

## Context

The process sampler registers the gauges exported through OpenTelemetry on
every run, so runtime data is a product concern, not a developer tool.
Users also need to see whether OTLP or profile export is enabled,
configured, started, or waiting on a restart. The UI frame sampler has a
different cost: its `requestAnimationFrame` loop keeps the webview from
idling.

## Decision

1. **Settings ▸ Observability is the user surface.** It shows the runtime
   dashboard and, for the OTLP and Profiles destinations, whether each is
   enabled, configured, running, and needs a restart. "Running" means the
   SDK pipeline started, not that the backend accepted data.

2. **`app.ObservabilityService` owns process sampling and export status.**
   It registers the process gauges for the process lifetime and combines the
   effective settings with the startup result (`TelemetryRuntime`) passed in
   from the composition root.

3. **Exporter changes apply after restart.** Telemetry is built in
   `cmd/desktop/main.go` before `app.App` exists, because the logger needs
   it, so the core receives a startup snapshot rather than the provider.

4. **Configuration stays file-owned.** The page has no destination or
   credential fields; it links to the guide for editing `settings.yaml`,
   where credentials are `env:`, `file:`, or `op://` references.

5. **UI frame sampling runs only while the dashboard is open.** This gives
   up history of jank in other views so the app can idle when nobody is
   reading it.

The developer-tools gate itself stays as in
ADR developer-tools-are-reachable-in-a-shipped-build-behind-a-setting; this
replaces its decisions 4 and 5.

## Consequences

- Every user can inspect CPU, memory, process-tree cost, Go runtime state,
  and UI frame behaviour without enabling developer tools.
- OTLP and Profiles are independent gates, but provider startup is
  all-or-nothing: a failure in either leaves both inactive, and the page
  shows the error.
- Delivery failures after startup stay in logs. The UI makes no
  connection-health claim the SDK cannot support.
- `perf.jsonl` gets long-frame and event-loop samples only while the
  dashboard is open.
