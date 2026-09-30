# Telemetry is exported over OTLP with no collector, and the same instruments serve a local scrape

- **Status:** accepted
- **Date:** 2026-09-05

## Context

The app observes itself four disconnected ways — zerolog to a file, pprof, UI
spans in `perf.jsonl`, and a `tmuxcc.MetricsSink` that has only ever been
handed `NopMetrics` — and none of them leaves the machine or can be compared
across two builds. Running a collector alongside a desktop app was rejected
outright: a second process to install and supervise, for one producer.

Binary size was the argument against the SDK, so it was measured. On this app
with production flags (39.0 MB baseline) the three signals cost **4.52 MiB
(+12.2%)** and 73 modules. The *shape* of that cost decided the scope: in
isolation the first signal costs about 17 MB and each additional one about
1 MB, because nearly all of it is one shared OTLP/protobuf/grpc base (grpc
links even into the HTTP-only exporters). Staging one signal now would have
paid the whole bill for a third of the value. The Prometheus reader adds
**244 KiB** on top, `client_golang` overlapping almost entirely with what the
exporters already pulled in.

## Decision

1. **OTLP/HTTP with gzip, straight to the endpoint.** `telemetry.endpoint` is
   the signal-less base and the per-signal paths are appended;
   `telemetry.instance_id` is the basic-auth username. No collector, no agent,
   one credential for all three signals. Prometheus remote write would have
   carried metrics alone and left traces and logs needing a second path.

2. **One MeterProvider, two readers.** `telemetry.enabled` adds a pushing
   periodic reader; `development.metrics.enabled` adds a Prometheus reader and
   mounts `/metrics` on the shared loopback server the way pprof does. The
   gates are independent, so a local scrape answers with no account configured.
   An instrument is declared once and both readers collect it, which is the
   point: a PromQL expression written against `curl localhost:PORT/metrics` is
   the same expression run against the backend. Both gates off returns the
   no-op object `Off` returns, following `perf.Off()`'s precedent.

3. **Four resource attributes, chosen for what survives as a queryable
   dimension.** A backend keeps only some as dimensions and files the rest into
   `target_info` (Prometheus) or structured metadata (Loki). `service.version`
   is promoted by Prometheus but **not** by Loki, which is why the release
   channel, not the version, is the primary separator: it is the only one of
   the two that selects a log stream.

   `deployment.environment.name` comes from the version string, and because
   `VERSION_LDFLAGS` applies only on the production branch of the platform
   Taskfiles, a working tree reports `source` and cannot land in a release's
   series without anyone configuring that.

   **Superseded in part by ADR telemetry-carries-the-opentelemetry-host-identifier-configured-for-the-machine.**
   An optional fifth resource attribute, `host.id`, now identifies the machine
   when the user configures it.

4. **The log bridge is a zerolog writer arm, not a `zerolog.Hook`.** A Hook is
   handed only the level and the message; an event's fields are not readable
   from a `*zerolog.Event`. Every arm of a `MultiLevelWriter` receives the
   encoded JSON event, which is what the existing `ConsoleWriter` arms parse to
   pretty-print. `NewLogger` therefore takes extra writers. The arm never fails
   a write: an error there would be an error about an error.

5. **`telemetry.token` is a reference, not a token** — `env:NAME`,
   `file:/path`, or `op://vault/item/field`, resolved through
   `internal/app/secrets` and rejected if it is a literal
   (ADR config-holds-secret-references-not-secrets-and-1password-is-one-of-the-sources). `settings.yaml` therefore names where the
   credential lives without carrying it, and stays safe to commit.

6. **The endpoint is validated for https, deliberately not for loopback.**
   `development.github.api_base` is pinned to loopback precisely so a persisted
   setting cannot aim the app at a remote host. This one is remote by
   definition, so the check that is left to make is that the credential does
   not cross the network in the clear.

## Consequences

- The shipped binary grows 4.52 MiB and `go.mod` gains 73 modules, including
  grpc, which nothing in this app calls. That is the price of the OTLP
  exporters and it is paid once for all three signals.
- Export failures are the SDK's default error handler, which writes to stderr.
  Routing them through zerolog would close a loop — an export error logged
  through the bridge that failed to export — so it is deliberately left alone.
- `otlploghttp` is experimental. The Go log API and SDK reached RC in
  v1.47.0-rc.1 but the exporters were left outside that RC's stability scope,
  so the log path is the one to expect churn from on an upgrade.
- The whole metric surface is Go runtime metrics and the whole trace is
  `app.startup`. `tmuxcc.MetricsSink` stays on `NopMetrics`: it is the
  interface that keeps the SDK out of `tmuxcc`, and a real implementation is
  the next change.

  **Superseded by ADR a-package-declares-its-own-opentelemetry-instruments-against-the-global-provider.** The consumer-defined interface was
  the wrong shape and `MetricsSink` is deleted. `telemetry.New` registers the
  providers globally, and a package declares its own instruments against them;
  the boundary this ADR meant to draw is the API/SDK split, not the package.
  Everything else here stands.
- `internal/app/perf` is untouched and still the only way to read UI spans
  without a backend. Replacing it with a JSONL `SpanExporter` beside the OTLP
  one is a separate decision.
