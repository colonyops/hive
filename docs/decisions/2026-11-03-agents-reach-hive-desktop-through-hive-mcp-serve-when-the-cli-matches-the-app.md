# Agents reach Hive Desktop through hive mcp serve when the CLI matches the app

- **Status:** accepted
- **Date:** 2026-11-03

## Context

Agent configurations held the desktop's loopback address with its port. A port change broke every configuration that named the old one, and a Code session's agent had to be wired by hand. The hive CLI already finds the desktop through its endpoint file, and since ADR every-operation-is-declared-once-as-a-spec-in-internal-ops-and-each-surface-is-built-from-it it compiles the same operation catalog the desktop serves.

## Decision

**`hive mcp serve` is a stdio MCP server that forwards catalog operations to the running desktop.**

- It serves the desktop-owned ops of the `--toolset`s it is given (default: all), with the desktop's names, descriptions and schemas.
- It removes `caller` from each schema and fills it from `--caller`, `HIVE_AGENT_SESSION`, or the working directory.
- It reads the endpoint file on every call, so a desktop restart needs no reconnect. While the desktop is down, a call answers `unavailable: Hive Desktop is not running` as a tool error.
- It does not open `hive.db` or start the engine.

**The desktop writes this form only when the CLI matches the app.** The three hive catalogue entries, and Settings ▸ MCP servers ▸ Add, use `hive --data-dir <dir> mcp serve --toolset <list>` when `hive` (`daemon.hive_path`, else the login-shell PATH) reports the same version as the app. Otherwise they keep the HTTP address. The toolset list comes from the catalog by `MCPPath`. A CLI of another version compiles another catalog, so a mismatch would advertise tools the desktop does not have.

**Exec operations are not part of this decision.** The catalog has none. They arrive with #500, together with the token they need.

## Consequences

- A user without the hive CLI, or with a CLI of another version, keeps the HTTP entry until #509 installs a matching `hive` with the app.
- A dev build reports `dev`, so it matches only a dev CLI.
- The probe runs `hive --version` once per binary path and modification time.
- The HTTP servers stay. They serve the stdio proxy, `hive dt`, and clients that cannot start a process.
