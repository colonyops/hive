# Telemetry carries the OpenTelemetry host identifier configured for the machine

- **Status:** accepted
- **Date:** 2026-09-18

## Context

Hive used `development.instance.id` as `service.instance.id`, but that value is
stable across launches and can be repeated on another machine. OpenTelemetry
requires the service instance id to be globally unique among concurrent
instances of one service. Installed builds also left it empty. A person
comparing telemetry from several machines therefore had no standard resource
identity to correlate their metrics, logs, traces, and profiles (ADR telemetry-is-exported-over-otlp-with-no-collector-and-the-same-instruments-serve-a-local-scrape).

OpenTelemetry defines [`host.id`](https://opentelemetry.io/docs/specs/semconv/resource/host/) as the unique host identifier. For a
non-containerized system it should be the machine id. `host.name` is a display
name and `service.instance.id` identifies a running service instance, so neither
has the requested meaning.

Hive must not collect a platform machine identifier without the user's choice.
The identifier can be sensitive, and `settings.yaml` may be shared between
machines.

## Decision

Add optional `telemetry.host_id` configuration, with
`HIVE_DESKTOP_TELEMETRY_HOST_ID` as its environment override. When set, export
its value as the OpenTelemetry `host.id` resource attribute. Do not derive it
from the hostname, hardware, or operating system.

Project the same value onto Pyroscope profiles as `host_id`, following the
existing underscore form for OpenTelemetry resource attributes. Omit empty
values from every signal.

Generate a random UUID v4 once when an enabled telemetry provider starts and
export it as `service.instance.id` across every active signal. Remove
`development.instance.id`; the settings v5 migration drops it from existing
strict YAML before decoding.

## Consequences

- One configured value correlates metrics, logs, traces, and profiles from the
  same machine.
- Users choose whether to disclose a stable machine identifier and how to name
  it.
- A dotfiles-managed `settings.yaml` needs a machine-specific value or the
  environment override. An omitted host id preserves existing telemetry.
- Every telemetry-enabled launch has a distinct service instance id without
  configuration. Machine correlation uses `host.id`, not that ephemeral id.
