# Agent-facing HTTP API on a unified loopback server

- **Status:** partly superseded by [mcp-replaces-the-agent-facing-http-api](2026-08-05-mcp-replaces-the-agent-facing-http-api.md)
- **Date:** 2026-07-27

> **Superseded in part (2026-08-05):** the *agent-facing* surface described
> below — inbox, feeds, profiles, actions, source refresh, flow dry runs — is
> the MCP server's now (ADR mcp-replaces-the-agent-facing-http-api), and the routes are deleted. What this ADR
> decided that still holds: **one loopback server** shared by the webhook
> listener, the HTTP adapter, pprof and now `/mcp`; the **loopback bind as the
> security boundary** for an unauthenticated agent surface; and the
> **single-writer** model. `/api/status` and `/api/version` survive as the
> plain-GET liveness probe. `/api/terminal/*` was never this ADR's subject —
> it is the Wails frontend's transport (ADR terminal-transport, ADR a-workspace-declares-its-own-authority).

> **Scope (amended 2026-07-27):** the "read + reload" framing below was the
> first slice, not the ceiling. This is the app's **agent-facing control
> surface** — an agent drives it to observe *and operate* the app without
> touching SQLite or the config files, and it grows toward full agentic
> control. The profile resource is the first mutation surface beyond a reload:
> `GET|POST /api/profiles`, `DELETE /api/profiles/{id}`, and
> `GET|PUT|DELETE /api/profiles/{id}/image`. Each runs the same core method the
> Wails UI does — create/delete a flow, set an avatar via the
> normalize-store-reference path — so an agent operates the app the supported
> way rather than editing flow YAML and the data dir by hand. The same core
> methods are what a future
> MCP adapter exposes as tools. The loopback bind and single-writer model are
> unchanged; each new mutation still maps its `app.Kind` to a status exactly
> once and holds no logic of its own.

## Context

Testing the notification pipeline against a live instance meant reading
`desktop-pipeline.db` directly (`inbox_item`, `webhook_capture`) to see what the
pipeline concluded, waiting up to a poll interval (≤60s) for an overlay to
surface, and enabling the webhook listener by hand. There was no supported,
agent-facing surface for *observing* the pipeline or *forcing* a re-poll — only
the SQLite schema, which a test then couples to.

`architecture.md` already names an `httpapi/` adapter (REST + SSE) as a target
that is "not yet built". This is the first slice of it, scoped to the harness
need: read the pipeline's conclusions and force a re-evaluation. The devserver
control API (ADR devserver-agent-control-api) is the *act* half — this is the *observe/arrange* half.

The webhook listener (ADR webhook-listener-placement) already owns a loopback server, and
`architecture.md` describes `httpapi` as "mounted via ServeHTTP at a Route" —
mounted onto a server, not owning one. So the question was not whether to share
a server but how to configure it, and how a test waits for a result.

## Decision

1. **One loopback HTTP server, two handlers.** A single `http` settings section
   (`enabled`, `host`, `port`) configures one loopback server that hosts both
   the webhook listener (`/hooks/…`) and the agent API (`/api/…`). There is no
   separate webhook-vs-API toggle: `webhook.Listener.MountAPI` mounts the API
   handler as an opaque `http.Handler`, `App.MountAPI` forwards to it, and
   `main.go` mounts it before `Start` whenever the listener exists. This
   replaces the earlier `webhooks` section and a separate `development.api`
   gate.

2. **On by default.** `http.enabled` defaults to true. The server is
   loopback-only (validated), so it is reachable only from this machine, which
   is the accepted trade for zero-setup local use — a fresh install and every
   dev worktree serve `/api/` and `/hooks/` with no manual step. This
   supersedes ADR local-webhook-listener's default-*off* posture for the listener; the port is
   still drawn at random per install and bound only to loopback.

3. **A driving adapter, `internal/adapter/httpapi`, over `*app.App`.** It maps
   `app.Kind` to HTTP status once and holds no logic of its own. Endpoints:
   `GET /api/{version,status,feeds,inbox,inbox/events}` and
   `POST /api/sources/refresh`. Transport-neutral helpers (JSON writing, the
   version/build handler) live in `internal/webtools`, shared with the
   devserver so both APIs report build identity in one shape.

4. **No server-side wait; the workflow is reload → read → retry.**
   `RefreshSources` drops the fetch caches, drives one producer tick, and
   returns its `{sources, appended, failed}` summary. The engine commits a
   moment later on its own goroutine, so a harness retries the read. A long-poll
   `waitFor` and a synchronous engine drain were both considered and rejected —
   see Alternatives.

5. **devtools bootstraps it.** `prepare` allocates a free port and writes
   `HIVE_DESKTOP_HTTP_ENABLED` and `HIVE_DESKTOP_HTTP_PORT` into `launch.env`,
   so a prepared worktree serves the API and webhooks at a known port.

## Consequences

- **One port, discovered once.** The agent reads the HTTP port from
  `launch.env`; `GET /api/status` confirms the webhook host/port/path so a
  devserver inline push target can be built from the same value, and
  `GET /api/version` confirms the build.
- **Shipped builds run a loopback server by default.** Any local process can
  reach `/api/` (inbox reads + a refresh trigger) and `/hooks/`. Accepted for
  local use; the loopback bind is the boundary.
- **The refresh answer is honest.** Because the tick returns a summary, a forced
  refresh reports "every source failed" instead of a blind 200.
- **Reads return slices.** An `external_id` is the source's own id (a GitHub
  global node id, not `repo#num`) and can exist across profiles and source
  scopes, so lookups return every match; the harness matches on the payload's
  `repo`/`num`/`reason`.
- **Interim layering.** The whole-app API is hosted by the webhook connector's
  listener; the webhook package only ever sees an opaque `http.Handler`, so when
  the full REST + SSE surface arrives it can graduate to an App-owned server
  without touching the webhook code. The Settings ▸ Webhooks UI still edits this
  server's enable/host/port and is a candidate for a later relabel.

## Alternatives considered

- **A second port + a `development.api` section.** Rejected: another listener,
  another settings block, and an extra value to discover, for no gain over the
  one loopback server the harness needs anyway.
- **Server-side `waitFor` long-poll.** Rejected: it puts real logic in the core
  for a concern a client retry handles, and reaching a *terminal* state can take
  more than one cycle (GitHub absence confirmation), which a client loop rides
  over naturally.
- **Synchronous engine drain** so `reload`/push block until committed. Rejected:
  the engine is deliberately single-goroutine and latch-driven; a
  block-until-committed path is invasive, still needs a separate settle for the
  webhook path, and still can't express "reached state X".
