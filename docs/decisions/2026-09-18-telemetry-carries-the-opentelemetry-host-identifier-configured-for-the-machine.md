# Telemetry carries the OpenTelemetry host identifier configured for the machine

- **Status:** accepted
- **Date:** 2026-09-18

## Context

Someone comparing telemetry from several machines needs a standard resource
identity to correlate metrics, logs, traces, and profiles
(ADR telemetry-is-exported-over-otlp-with-no-collector-and-the-same-instruments-serve-a-local-scrape).
OpenTelemetry defines [`host.id`](https://opentelemetry.io/docs/specs/semconv/resource/host/)
for that. `service.instance.id` must be unique among concurrent instances, so
a stable configured value cannot fill it.

Hive must not collect a platform machine identifier without the user's choice:
it can be sensitive, and `settings.yaml` may be shared between machines.

## Decision

`telemetry.host_id` in `settings.yaml` (environment override
`HIVE_DESKTOP_TELEMETRY_HOST_ID`) is optional. When set, it is exported as the
`host.id` resource attribute and as the Pyroscope label `host_id`. Hive never
derives it from the hostname, hardware, or operating system. Empty values are
omitted from every signal.

`service.instance.id` is a random UUID v4 generated once when an enabled
telemetry provider starts, shared by every active signal. Settings version 5
removed the old `development.instance.id` key.

## Consequences

- A dotfiles-managed `settings.yaml` needs a machine-specific value or the
  environment override.
- Machine correlation uses `host.id`, never the per-launch service instance id.
