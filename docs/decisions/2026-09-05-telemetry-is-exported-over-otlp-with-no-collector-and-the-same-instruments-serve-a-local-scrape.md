# Telemetry is exported over OTLP with no collector, and the same instruments serve a local scrape

- **Status:** accepted
- **Date:** 2026-09-05

## Context

Hive Desktop needs metrics, traces and logs that leave the machine and can
be compared across builds. Running a collector beside a desktop app means a
second process to install and supervise for one producer, so the app
exports directly. Nearly all of the SDK's binary cost is one shared
OTLP/protobuf base, so adopting one signal costs almost as much as all
three; the app takes all three at once.

## Decision

1. **OTLP/HTTP with gzip, straight to the endpoint.** `telemetry.endpoint`
   is the signal-less base and the per-signal paths are appended;
   `telemetry.instance_id` is the basic-auth user. One credential covers all
   three signals.

2. **One MeterProvider, two readers.** `telemetry.enabled` adds a pushing
   periodic reader; `development.metrics.enabled` adds a Prometheus reader
   and mounts `/metrics` on the loopback server. The gates are independent,
   so a local scrape works with no account. An instrument is declared once
   and both readers collect it, so a PromQL expression written against the
   local scrape runs unchanged against the backend. With both gates off,
   `telemetry.Off()` registers nothing.

3. **Resource attributes are chosen for what survives as a queryable
   dimension**: `service.name`, `service.version`, `service.instance.id`,
   `deployment.environment.name`, and the optional `host.id`
   (ADR telemetry-carries-the-opentelemetry-host-identifier-configured-for-the-machine).
   Loki does not promote `service.version`, so the release channel in
   `deployment.environment.name` is the primary separator. A build without
   release ldflags reports `source` and cannot land in a release's series.

4. **The log bridge is a zerolog writer arm, not a `zerolog.Hook`.** A hook
   sees only the level and message, while a `MultiLevelWriter` arm receives
   the encoded event with its fields. The arm never fails a write.

5. **`telemetry.token` is a reference, not a token**, resolved through
   `internal/platform/secrets` (ADR config-holds-secret-references-not-secrets-and-1password-is-one-of-the-sources).

6. **The endpoint must be https, not loopback.** It is remote by definition,
   so what is left to enforce is that the credential does not cross the
   network in the clear.

## Consequences

- Export failures go to the SDK's default error handler (stderr). Routing
  them through zerolog would loop an export error back through the bridge
  that failed.
- `otlploghttp` is outside the Go SDK's stability guarantees, so the log
  path is the one to expect churn from on an upgrade.
- UI spans stay in `perf.jsonl` (`cmd/desktop/internal/app/perf`); replacing
  that with a span exporter is a separate decision.
- How packages declare instruments is ADR a-package-declares-its-own-opentelemetry-instruments-against-the-global-provider.
