# The MCP server replaces the agent-facing HTTP API

- **Status:** accepted
- **Date:** 2026-08-05

## Context

The agent that drives Hive Desktop is an external CLI agent running in an
agent workspace, reaching the app through its generated `.mcp.json`
(ADR agent-workspace-sessions-are-tmux-sessions). Keeping a REST surface for
agents beside an MCP server would mean two descriptions of the same core
methods, and every new capability added twice or missing from one. This
supersedes the agent-facing half of ADR agent-http-api.

## Decision

**The MCP server is the agent-facing surface; the agent REST surface and its
OpenAPI document are deleted.** `internal/adapter/mcpsrv` serves tools over
Streamable HTTP at `/mcp`, on the same loopback server as the webhook listener.
It is a sibling of `httpapi`: a driving adapter with a tool table and thin
handlers over `*app.App`.

- **Its own tool table**, not a projection of httpapi's HTTP-shaped `Op` table.
  Input schemas are inferred from each handler's typed input struct, so a tool
  cannot advertise a field its handler does not accept.
- **Stateless, plain JSON responses** (`Stateless: true, JSONResponse: true`).
  A stateful server holds SSE streams that `http.Server.Shutdown` waits on, and
  no tool needs a server-initiated request.
- **Reads and safe mutations only; session control is excluded.** Starting a
  terminal or agent session is command execution, which the `/api/terminal/`
  bearer token guards (ADR terminal-transport).
- **Unauthenticated behind the loopback bind.** The rule is that command
  execution authenticates, and this surface has none. The SDK's DNS-rebinding
  protection stays on.
- **On whenever `http.enabled` is**, with no experimental gate.

Three contracts bind every tool:

1. An id that resolves to nothing is `not_found`, never an empty collection.
2. A mutation's answer reflects what happened, never a constant.
3. A response carrying source-supplied JSON takes a `detail` argument, defaults
   to the safe level, and echoes the level it used. `summary` and `full` mean
   the same on every tool.

Errors render an `app.Error`'s wrapped cause, not its `Msg` prefix alone.

**What stays on HTTP:** `/api/terminal/*`, the frontend's own transport, and
`/api/status` and `/api/version` for shell scripts and health checks. The MCP
`get_status` tool overlaps `/api/status` on purpose.

**Workspace wiring:** `mcpcatalog` has a `hive-desktop` entry with
`RuntimeURL: true`, because the loopback port is allocated at startup. The live
endpoint is substituted when the catalogue renders, and an unresolvable entry
reports a problem instead of rendering a dead address.

## Consequences

- A `jsonschema` struct tag means different things to the MCP SDK's inferrer
  (`google/jsonschema-go`, plain description) and to `invopop/jsonschema`
  (`enum=...,description=...`, used for connector config). Store view types
  therefore cannot be MCP output types, and tools that answer with them omit an
  output schema. Adapter-local input types use SDK-style tags.
- `store.Msg` cannot be a tool input: its `json.RawMessage` payload infers as an
  array. `execute_flow` takes an adapter-local `flowMessage` instead.
- A bad struct tag is a startup panic (`mcp.AddTool`), not a compile error.
  Every `mcpsrv` test builds the server through the same constructor, so the
  suite is the guard.
- New agent capabilities go on MCP, not `/api`.
- Images cross as base64; image reads answer with an MCP image content block.
