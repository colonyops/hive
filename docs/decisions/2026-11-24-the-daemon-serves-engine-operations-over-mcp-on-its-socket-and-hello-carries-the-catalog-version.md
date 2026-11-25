# The daemon serves engine operations over MCP on its socket, and Hello carries the catalog version

- **Status:** proposed
- **Date:** 2026-11-24

## Context

A server's daemon served only `SessionService` and its control calls, so `--server` reached sessions and nothing else. The engine operations already run from one handler table (ADR engine-operations-run-from-one-handler-table-that-the-desktop-mounts-and-the-hive-cli-calls-in-process). Generating Connect services from the catalog would add about 20 RPCs and 40 messages, a name mapping for each string enum, and a second validation path beside `mcpops.Mount`.

## Decision

**The daemon mounts the engine handler table as an MCP server at `/mcp` on the mux it already serves, and a client calls it over the same connection as its Connect calls.**

- The server is stateless with JSON responses, so a call works on any new ssh connection. It serves every engine operation outside the `sessions` and `session-control` toolsets, which `SessionService` and the fleet already serve, over the daemon's own engine with no fleet.
- `daemonclient.Client.CallEngine` sends one MCP call through the client's `http.Client`, so `ssh -- hive daemon dial-stdio` carries it with no change. A read gets the read timeout; a change has no bound, as for sessions.
- **`Hello` carries `catalog_version`.** A client calls `wire.MinCatalogVersion` to `wire.CatalogVersion` and refuses an engine call outside that range with an error that names both hive versions. A daemon from before this sends 0 and serves no engine operations. The catalog range gates engine calls only, and `ProtocolVersion` does not change, so a client and a daemon of the previous release keep their session calls (ADR a-client-refuses-a-remote-daemon-outside-its-protocol-range). Raise `CatalogVersion` for a change that breaks an older peer's engine calls, such as a renamed tool or a changed field meaning, and `MinCatalogVersion` too when the client cannot work around it.
- **One routing rule.** `engineops.Handlers.CallOn` runs a call on `--server`, else on the server of a `<server>:<id>` caller; both set and different is `invalid`. On a server the caller is the id of a qualified caller, `global` for a path, and a bare id as given. Every generated engine command takes `--server` and `HIVE_SERVER`, as the session commands do.
- The daemon watches Drive, because the server's CLI and agents write documents beside it.

## Consequences

- A new engine operation needs no wire change. An older hive of the same catalog lacks it and answers `unavailable` with both hive versions; a new input field that an older hive does not know is refused as `invalid`, and the CLI sends a field only when its flag is set.
- A remote answer is decoded into the handler's own type. Fields that do not cross as JSON are set again on the client: `blocked` from the listed tasks, and a message wait from the request.
- `hive mcp serve` and the desktop do not reach servers for these operations yet: their tools have no `server` field.
- A remote caller that is a path acts as `global` on the server, so `task_next --assign` on a server needs `--caller <server>:<session id>`.
