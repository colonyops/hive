# Continuous profiles are pushed directly with Pyroscope

- **Status:** accepted
- **Date:** 2026-09-17

## Context

OpenTelemetry Profiles is Alpha and the Go API and SDK do not implement it.
Hive exports its other telemetry directly from the desktop process with no
collector (ADR telemetry-is-exported-over-otlp-with-no-collector-and-the-same-instruments-serve-a-local-scrape).
Grafana Cloud Profiles has its own endpoint and basic-auth user, separate
from OTLP.

## Decision

Use `grafana/pyroscope-go` in push mode. `telemetry.profiles` has its own
enable flag, endpoint, user, and token reference, and runs with OTLP export
off. `cmd/desktop/internal/app/telemetry` owns its lifecycle beside the OTel
providers.

Collect CPU and the allocation and in-use heap profiles, with
`DisableGCRuns` so collection never forces a garbage collection. Goroutine,
mutex and block profiles wait until measurements justify their volume.

While continuous profiling is active, `/debug/pprof/profile` uses
Pyroscope's compatible CPU handler, since two CPU profilers cannot run at
once. The SDK's HTTP client is bounded, and its context-free `Stop` runs
behind an adapter that honours the shutdown context, so shutdown stays
within the telemetry grace. `PYROSCOPE_ADHOC_SERVER_ADDRESS` is rejected,
because the SDK applies it after Hive validates the endpoint and would send
the configured credentials to it.

## Consequences

- Profile labels match the OTel resource: service version, deployment
  environment, instance id, and the optional host id. Empty labels are
  omitted.
- Heap profiles can include objects not yet collected, in exchange for no
  forced GC work every upload interval.
- Upload failures never stop the app. Shutdown can lose the final profile
  interval rather than hold the process open.
- OTLP Profiles can replace this transport once the Go SDK implements it.
