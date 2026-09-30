# pprof debug endpoint on the shared loopback HTTP server

- **Status:** accepted
- **Date:** 2026-07-27

## Context

`development.pprof` has been typed and defaulted off since ADR desktop-configuration, but
nothing started the endpoint. The open question was where the
`net/http/pprof` handlers bind: a standalone listener with its own lifecycle,
or mounted on the always-on loopback HTTP server (ADR agent-http-api) that already hosts
the webhook listener (`/hooks/…`) and the agent API (`/api/…`).

A standalone `pprofsrv.Server` was built first — its own `http.Server`,
`Start`/`Stop` behind a `stopOnce`, App-owned lifecycle wiring, and a package
of tests. That is a whole second service to stand up and tear down for a
handful of stdlib handlers that profile the Go runtime and touch nothing in
`app.App`.

## Decision

**Mount the pprof handlers on the shared loopback HTTP server, gated by
`development.pprof`.** There is no dedicated server and no new lifecycle: the
handler is one more mount on the server ADR agent-http-api already owns and ADR webhook-listener-placement
already gives an idempotent shutdown.

- **`httpapi.PprofHandler()`** returns the `/debug/pprof/…` handlers on a
  private mux (never `http.DefaultServeMux`, which importing `net/http/pprof`
  writes to). It lives in the HTTP adapter beside the agent API — the same
  place, the same server.
- **`webhook.Listener.MountAPI` now takes any number of mounts** (a `[]mount`
  instead of one slot), so the agent API at `/api/` and pprof at
  `/debug/pprof/` are siblings on the one mux. `main.go` mounts pprof before
  `Start` only when `development.pprof.enabled`, so by default there is no
  route at all — not even a disabled one.
- **`PprofSettings` is `{ enabled }` only.** The former `host`/`port` were the
  standalone design's; on the shared server the address is `http.host` /
  `http.port`, so a separate pair could only mislead. They are removed rather
  than left as dead config.

This makes the coupling explicit: pprof requires the HTTP server. When
`http.enabled` is false (or the server is skipped, as in mock mode) there is
nothing to mount onto, so `development.pprof` has no effect. That is acceptable
for a debug endpoint and is the price of not running a second listener.

## Consequences

- No `internal/app/pprofsrv` package and no lifecycle branch: enabling pprof is
  a conditional mount in `main.go`, torn down with the shared server it rides.
- `MountAPI` growing from one handler to many is a small generalisation of what
  ADR agent-http-api introduced, and the next adapter that wants the loopback port mounts
  the same way.
- pprof is reachable at `http://<http.host>:<http.port>/debug/pprof/` when
  enabled; the bound port is already logged by the webhook listener at startup.
- Reversing this — a separate pprof port again — means reintroducing the
  standalone server this ADR removed, and belongs in a superseding decision.
