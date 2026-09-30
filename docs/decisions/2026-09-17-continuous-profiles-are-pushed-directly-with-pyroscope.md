# Continuous profiles are pushed directly with Pyroscope

- **Status:** accepted
- **Date:** 2026-09-17

## Context

OpenTelemetry Profiles is Alpha and the Go API and SDK do not implement it. Hive already exports its other telemetry directly from the desktop process, with no collector to install or supervise (ADR telemetry-is-exported-over-otlp-with-no-collector-and-the-same-instruments-serve-a-local-scrape).

Grafana Cloud Profiles has a separate endpoint and basic-auth user from Grafana Cloud OTLP. Continuous CPU profiling also conflicts with the standard `/debug/pprof/profile` handler, and the Pyroscope SDK's context-free `Stop` can otherwise exceed the desktop's two-second telemetry shutdown budget.

## Decision

Use `grafana/pyroscope-go` in push mode and give `telemetry.profiles` an independent enable flag, endpoint, user, and token reference. The profile exporter can run with OTLP export disabled. `internal/app/telemetry` owns its process-wide lifecycle beside the OTel providers.

Collect CPU and the standard allocation and in-use heap profiles. Set `DisableGCRuns` so profile collection does not force garbage collection in the desktop process. Do not collect goroutine, mutex, or block profiles until measurements justify their volume and runtime-wide sampling controls.

Use Pyroscope's compatible CPU handler for `/debug/pprof/profile` while continuous profiling is active. Bound the SDK's HTTP client to the telemetry shutdown grace and run its context-free stop behind a context-selecting adapter. Reject `PYROSCOPE_ADHOC_SERVER_ADDRESS` because the SDK applies it after Hive validates the endpoint while retaining the configured credentials.

Do not add trace-to-profile correlation in this change. It requires another SDK and Grafana data-source configuration, while profile export is useful and testable without it.

## Consequences

- The stripped production server binary grows from 44,211,266 to 49,365,298 bytes: 4.92 MiB, or 11.7%.
- Profile credentials follow the existing secret-reference rules and never persist as token values.
- Static profile labels match the OTel service version, deployment environment, and instance identity. Empty labels are omitted.
  **Superseded in part by ADR telemetry-carries-the-opentelemetry-host-identifier-configured-for-the-machine:**
  profiles now carry the optional host identity too.
- Heap profiles can include objects not yet removed by garbage collection, in exchange for avoiding forced GC work every upload interval.
- Runtime upload failures do not stop the app. Shutdown can lose the final profile interval instead of holding the process open.
- OTLP Profiles can replace the Pyroscope-specific transport when the Go SDK provides a stable implementation.
