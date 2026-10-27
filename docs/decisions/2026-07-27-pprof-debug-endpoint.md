# pprof debug endpoint on the shared loopback HTTP server

- **Status:** accepted
- **Date:** 2026-07-27

## Context

`development.pprof` needed somewhere to bind the `net/http/pprof` handlers:
a standalone listener with its own lifecycle, or the loopback HTTP server that
already hosts the webhook listener and the app's other local surfaces (ADR
agent-http-api). A standalone server is a whole second service to start and
stop for a handful of stdlib handlers.

## Decision

**Mount pprof on the shared loopback HTTP server, gated by
`development.pprof.enabled`.**

- `httpapi.PprofHandler` serves `/debug/pprof/` on a private mux, never
  `http.DefaultServeMux` (which importing `net/http/pprof` writes to).
- `cmd/desktop/main.go` mounts it through `App.MountAPI` only when enabled, so
  by default the route does not exist.
- `PprofSettings` is `{ enabled }` only. The address is `http.host` and
  `http.port`.

## Consequences

- pprof requires the HTTP server. When `http.enabled` is false, or the server
  is skipped (mock mode), `development.pprof` has no effect.
- pprof is at `http://<http.host>:<http.port>/debug/pprof/` when enabled.
