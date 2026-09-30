# The MCP server replaces the agent-facing HTTP API

- **Status:** accepted
- **Date:** 2026-08-05
- **Supersedes:** [agent-http-api](2026-07-27-agent-http-api.md) (the agent-facing half), [self-describing-agent-api](2026-07-27-self-describing-agent-api.md)

## Context

`architecture.md` has reserved `internal/adapter/mcpsrv/` for "tools over App"
since the migration path was written, and ADR agent-http-api's own amendment said the
core methods it exposed over REST "are what a future MCP adapter exposes as
tools". The HTTP surface was always the placeholder.

Issue #79 scoped that adapter as one tool table serving two consumers: an
in-memory transport for an embedded agent (#76) plus an optional external
endpoint. **That premise no longer holds.** The Agents area shipped as a layer
over a Claude Code session (ADR a-workspace-declares-its-own-authority, ADR agent-workspace-sessions-are-tmux-sessions), not as an embedded agent, so
nothing in this process wants an in-memory MCP client. The agent that needs
these tools is external — the CLI agent running inside an agent workspace,
reaching the app through its generated `.mcp.json`.

Meanwhile the REST surface had acquired a real cost. It is the thing agents are
told to use: a shipped skill, a settings prompt, and the repo's own dev tooling
all pointed at `/api` and its OpenAPI document. Keeping it beside an MCP server
would mean maintaining two descriptions of the same core methods and telling
agents two stories about how to drive the app — and every capability added
afterwards would have to be added twice or be silently missing from one.

## Decision

> **Amended 2026-08-05, after the surface was exercised end to end.** Three
> contracts were added, each closing a case where a tool answered a question
> the caller had not asked. They now bind every tool, not only the ones that
> were wrong:
>
> 1. **An id that resolves to nothing is `not_found`, never an empty
>    collection.** A typo, a real-but-empty scope and a nonexistent one were
>    one indistinguishable response. Where a scope can legitimately outlive its
>    declaration — inbox rows whose profile was deleted — the read still runs
>    first and the emptiness is only *explained* afterwards, so tightening the
>    contract never hides state.
> 2. **A mutation's answer is never a constant.** `delete_profile` returned
>    `{"deleted": true}` unconditionally, which on the one irreversible tool is
>    indistinguishable from having destroyed the wrong thing.
> 3. **A response carrying source-supplied JSON takes a `detail` argument and
>    defaults to the level that is safe against real data.** This surfaced
>    twice — `execute_flow` reporting every message once per node per
>    direction, and `list_inbox` returning 150KB for a 47-item feed — because
>    it is one problem: the size of an inbox item's payload, an event's detail
>    and a dry run's messages is set by the source, not by anything here, and
>    every listing multiplies it. A cap would truncate silently; the level is
>    the caller's to raise, and each answer echoes the one it used so an
>    omitted field is never read as an absent one. `summary` and `full` mean
>    the same thing on every tool; a tool may add rungs between them.
>
> The adapter also renders an `app.Error`'s wrapped cause rather than its `Msg`
> alone — the split `MarshalJSON` makes for the frontend. `Msg` is written as a
> context prefix, so dropping the cause hands an agent a dangling phrase with
> no reason in it, and the consumer here is a model debugging its own call on
> the user's machine.

**The MCP server is the agent-facing surface. The REST surface that served that
role is deleted, not deprecated.**

`internal/adapter/mcpsrv` serves the desktop's capabilities as MCP tools over
Streamable HTTP at `/mcp`, on the same loopback server that hosts the webhook
listener (ADR agent-http-api's one-server rule is unchanged). It is a sibling of
`httpapi`: a driving adapter with no interface, a tool table carrying metadata
only, and handlers that are thin calls into `*app.App`.

Five choices are load-bearing.

**The tool table is its own, not a projection of httpapi's operations table.**
The two adapters duplicate prose and nothing else. `Op` is HTTP-shaped — path
parameters, query structs, raw binary bodies, `errchain` handlers — and
projecting a tool vocabulary onto it would couple the frontend's terminal
transport to the agent surface. Input schemas are inferred from each handler's
typed input struct by the SDK, so a tool cannot advertise a field its handler
does not accept.

**The server is stateless.** Every request carries its own short-lived session.
This is not a performance choice: it is what lets the adapter join no lifecycle
at all. A stateful Streamable HTTP server holds long-lived SSE streams that
`http.Server.Shutdown` waits on rather than closes — the hazard ADR terminal-transport
records for the terminal WebSocket — and the SDK's handler exposes no close.
Stateless removes the problem instead of managing it, and no tool needs a
server-initiated request. Responses are plain JSON rather than SSE for the same
reason, which also keeps the surface reachable with `curl`.

**The tool set is reads and safe mutations; session control is excluded.**
Starting a terminal or agent-workspace session is arbitrary command execution,
and that is exactly what the `/api/terminal/` prefix's bearer token exists to
guard (ADR terminal-transport, ADR a-workspace-declares-its-own-authority). Keeping it out is what lets this surface stay
unauthenticated.

**This surface is unauthenticated behind the loopback bind**, matching the
posture of the REST surface it replaces (ADR agent-http-api). It is a replacement, not an
escalation: the threat model — any local process — is unchanged, and a token
that any local process could read from the app's own state would be theatre.
The line the codebase already draws is *command execution authenticates*, and
this surface has none. DNS-rebinding protection stays on (the SDK's default
rejects a loopback request carrying a non-loopback `Host`), because a browser
is the one caller that could be induced to reach it without being a local
process.

**It is on whenever `http.enabled` is.** No `experimental.*` gate: it replaces
an on-by-default surface, and shipping it dark would leave agents with no way
to drive the app at all.

### What stays on HTTP

`/api/terminal/*` — the tmux, pop-up-terminal and agent-workspace control
planes and their WebSocket data planes — is the Wails frontend's own transport,
driven by hand-written TypeScript clients. It never was an agent surface.

`/api/status` and `/api/version` stay too. They answer "is the app up, and
which build is it?", which a shell script or a health check asks with a plain
GET; a JSON-RPC handshake is the wrong shape for a liveness probe. The MCP
`get_status` tool deliberately overlaps `/api/status`: the route serves shell
tooling, the tool serves an agent that should not have to shell out. That is a
considered duplication of a two-field answer, not drift.

### Workspace wiring

`mcpcatalog` gains a `hive-desktop` entry, so a workspace declaring
`mcps: [hive-desktop]` gets these tools in its generated `.mcp.json`. Its URL
cannot be static — the loopback port is allocated at startup — so the
descriptor carries `RuntimeURL: true` and no URL, and the live endpoint is
substituted when the catalogue is rendered. An entry that cannot be resolved
reports a problem rather than rendering an address nothing answers, the same
posture `problemFor` takes for a stdio command that is not on `PATH`.

## Consequences

- **The OpenAPI document is gone**, and with it `pb33f/libopenapi`,
  `libopenapi-validator`, and `gorilla/schema`. ADR self-describing-agent-api's self-description
  argument is fully honoured by `tools/list`, which is generated from the same
  Go types and needs no second document to validate. `invopop/jsonschema`
  remains for connector config schemas; the MCP SDK brings its own reflector.
- **A `jsonschema` struct tag means something different on each side.** The
  SDK's inferrer (`google/jsonschema-go`) treats the tag as a plain description
  and *rejects* one beginning with `WORD=`, which is exactly the
  `enum=…,description=…` form invopop uses. Store view types therefore cannot
  be MCP output types; tools answering with them omit their output schema,
  which costs nothing since output schemas are advisory. Adapter-local input
  types carry SDK-style tags and infer in full.
- **`store.Msg` cannot be a tool input type.** Its `json.RawMessage` payload
  infers as an array schema, so a real JSON-object payload would be rejected
  before reaching the handler. `execute_flow` takes an adapter-local
  `flowMessage` with `any` payloads and converts at the seam — a
  transport-shaped DTO where the architecture already says one belongs.
- **A bad struct tag is a startup panic, not a compile error**, because
  `mcp.AddTool` panics on a type it cannot infer. Every `mcpsrv` test builds
  the server through the same constructor, so the suite is the guard.
- **The shipped `hive-http-api` skill is now `hive-mcp`**, and the repo's own
  `desktop-api` skill is rewritten around tools. Anything still pointed at
  `/api/inbox` gets a 404 rather than a stale answer — deliberate, and pinned
  by a test.
- **A route added to the agent API while this was in flight moves with the
  rest.** ADR an-item-session-link-is-desktop-state-keyed-on-item-coordinates's `GET /api/inbox/sessions` landed on main after this branch
  was cut; it is `list_item_sessions` here. Nothing else consumed it — the
  detail pane reads the same core method over its Wails binding — so the route
  is deleted rather than kept, and the capability is not lost. Any further
  agent capability written against `/api` before this merges has to make the
  same trip.
- **Images cross as base64 rather than raw bodies.** These are 128×128 PNGs and
  the Wails binding already ships base64 for the same operation, so nothing
  regresses; image *reads* answer with an MCP image content block, which is
  strictly better than the old binary GET for a model that can see.
- The `Op` table no longer feeds a generated document, so its `Summary` and
  `Errors` fields now only document routes where they are declared. They are
  kept because the terminal surface is large and the prose is the only
  description it has.
