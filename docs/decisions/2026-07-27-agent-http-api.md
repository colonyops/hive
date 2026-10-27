# Agent-facing surfaces on a unified loopback server

- **Status:** accepted
- **Date:** 2026-07-27

## Context

The desktop needs local HTTP surfaces beside the webhook listener: a way for
an agent or test harness to observe and drive the app without reading
`desktop-pipeline.db`, a liveness probe, pprof, metrics, and the frontend's
terminal transport. The webhook listener (ADR webhook-listener-placement)
already owns a loopback server, so the question was how to share it rather
than whether to open more ports.

## Decision

1. **One loopback HTTP server hosts every local surface.** A single `http`
   settings section (`enabled`, `host`, `port`; env `HIVE_DESKTOP_HTTP_*`)
   configures it. Adapters mount onto it through `App.MountAPI`, which
   forwards to `webhook.Listener.MountAPI` as an opaque `http.Handler`, before
   `Start` (`cmd/desktop/main.go`). The webhook package never sees what it
   hosts.

2. **On by default, loopback only.** `http.enabled` defaults to true and
   `http.host` must be a loopback address (validated). Port `0` means draw one
   at first start; the app persists the allocated port unless the env var
   overrides it. This replaces ADR local-webhook-listener's default-off
   posture.

3. **The loopback bind is the security boundary** for the unauthenticated
   surfaces. Surfaces that spawn processes carry their own token (ADR
   terminal-transport, ADR a-workspace-declares-its-own-authority).

4. **Adapters are thin.** Each maps `app.Kind` to its transport's error shape
   once and holds no logic of its own; the logic lives in `*app.App`.

The agent-facing control surface that started here is the MCP server at
`/mcp` now (ADR mcp-replaces-the-agent-facing-http-api). What stays on
`httpapi` is `GET /api/status` and `GET /api/version` as the plain-GET
liveness probe, plus the terminal transport.

5. **devtools bootstraps it.** `prepare` writes `HIVE_DESKTOP_HTTP_ENABLED`
   and `HIVE_DESKTOP_HTTP_PORT` into `launch.env`
   (`cmd/desktop/devtools/instance.go`), so a prepared worktree serves at a
   known port.

## Consequences

- Shipped builds run a loopback server by default. Any local process can
  reach its unauthenticated routes; that is the accepted trade for zero-setup
  local use.
- A new local surface mounts on this server. It does not open a port or add a
  settings section of its own.
