# Agents reach Hive Desktop through hive mcp serve when the CLI matches the app

- **Status:** accepted
- **Date:** 2026-11-03

## Context

Agent configurations held the desktop's loopback address with its port. A port change broke every configuration that named the old one, and a Code session's agent had to be wired by hand. The hive CLI already finds the desktop through its endpoint file, and since ADR every-operation-is-declared-once-as-a-spec-in-internal-ops-and-each-surface-is-built-from-it it compiles the same operation catalog the desktop serves.

## Decision

**`hive mcp serve` is a stdio MCP server for the operation catalog. It runs engine operations itself and forwards desktop operations to the running desktop.**

- It serves the desktop-owned ops of the `--toolset`s it is given (default: all), with the desktop's names, descriptions and schemas.
- It removes `caller` from each schema and fills it from `--caller`, `HIVE_AGENT_SESSION`, or the working directory.
- **Engine operations run in-process** (#796), on the handler table the desktop mounts (ADR engine-operations-run-from-one-handler-table-that-the-desktop-mounts-and-the-hive-cli-calls-in-process), so they work while the desktop is closed. The session handlers reach the fleet as the CLI's session commands do: through the daemon when `experimental.daemon` is on (ADR a-hive-daemon-owns-session-writes-and-agent-status-behind-an-experimental-flag), and through every server under `servers:`.
- **`hive.db` opens on the first engine call and stays open** until the server exits. A server with only desktop toolsets never opens it. An open that fails answers `unavailable` and is tried again on the next call. The open runs the migrations every hive command runs. A long-lived server is like the TUI: a newer desktop that migrates the database later does not restart it, so migrations must stay additive.
- **`session-control` stays forwarded.** Its server is token-guarded, and the token is the grant a workspace holds for it (ADR session-operations-route-through-the-server-fleet-and-a-generated-command-keeps-the-spellings-of-the-command-it-replaces). It needs the running desktop.
- **A Drive call from a chat goes to the desktop**, as `hive drive` does: chat records live in the desktop's store, so only the desktop can file a chat's documents.
- Desktop operations go to the desktop. It reads the endpoint file on every call, so a desktop restart needs no reconnect. While the desktop is down, a call answers `unavailable: Hive Desktop is not running` as a tool error.

**The desktop writes this form only when the CLI matches the app.** The three hive catalogue entries, and Settings ▸ MCP servers ▸ Add, use `hive --data-dir <dir> mcp serve --toolset <list>` when `hive` (`daemon.hive_path`, else the login-shell PATH) reports the same version as the app. Otherwise they keep the HTTP address. The toolset list comes from the catalog by `MCPPath`. A CLI of another version compiles another catalog, so a mismatch would advertise tools the desktop does not have. An engine toolset that both servers offer goes in the stdio form whenever it is written: that form keeps working while the desktop is closed.

**Exec operations are not part of this decision.** The catalog has none. They arrive with #500, together with the token they need.

## Consequences

- A user without the hive CLI, or with a CLI of another version, keeps the HTTP entry until #509 installs a matching `hive` with the app.
- A dev build reports `dev`, so it matches only a dev CLI.
- The probe runs `hive --version` once per binary path and modification time.
- The HTTP servers stay. They serve the stdio proxy, `hive dt`, and clients that cannot start a process.
