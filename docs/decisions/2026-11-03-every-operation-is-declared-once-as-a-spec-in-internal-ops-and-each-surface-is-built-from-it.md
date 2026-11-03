# Every operation is declared once as a spec in internal/ops and each surface is built from it

- **Status:** accepted
- **Date:** 2026-11-03

## Context

The desktop declared its 51 MCP tools by hand in `mcpsrv`, and `hive dt`
copied eight of them by hand, with the split bounds and tab placements copied
a third time. Nothing failed when the copies drifted, and every new capability
was a tool that the CLI did not have. This supersedes the hand-written tool
table of ADR mcp-replaces-the-agent-facing-http-api. Its transport, posture
and three contracts stand.

## Decision

**An operation is an `ops.Spec[In]` in `internal/ops/<toolset>`, and each
surface is generated from it.** The desktop mounts a spec with
`mcpops.Mount(srv, spec, handler, mapErr)`. The hive CLI builds
`hive dt <resource> <verb>` from the same spec at compile time.
`internal/ops/catalog.All()` lists every spec.

- **The spec is name, owner, effect, toolset, input type, summary, guidance,
  positional args and constraints.** The output type is not part of it: output
  types are desktop types, and the CLI only prints JSON, so the binder supplies
  `Out`.
- **Owner** is `engine` or `desktop`. **Effect** is `read`, `write` or `exec`;
  `exec` is for an operation that runs a command the caller supplies, and no
  desktop operation does. `read` sets the MCP read-only hint.
- **A toolset** names the MCP path that serves it (`layout` and `drive` on
  `/mcp/drive`; `inbox`, `sessions`, `actions`, `diagnostics`, `flows` and
  `workspaces` on `/mcp`). It is metadata. Grants stay per server
  (ADR a-workspace-declares-its-own-authority).
- **Names are noun first.** A spec name is `<resource>.<verb>`, the MCP tool is
  `<resource>_<verb>`, and the command is `hive dt <resource-kebab> <verb>`. The
  verb comes from the closed list in `ops.Verbs`; `put` creates or replaces.
  The argument that says who is calling is `caller`, and a profile id is
  `profileId` on every operation.
- **Constraints are typed values on the spec** (`ops.Constraints`: enum, range,
  default), not struct tags. A key that names no field fails `InputSchema`, so a
  typo fails a test instead of silently doing nothing. A program that needs a
  bound imports the constant from the toolset package.
- **`Mount` validates arguments against the schema it advertises**, and a
  violation is `invalid`. The SDK's own check reports a violation with no kind
  in front of it.
- **The CLI exit code follows the kind of a refused call:** invalid 2,
  not_found 3, conflict 4, unavailable 5, unauthenticated 6, any other 1
  (`ops.ExitCode`). `hive dt` reads the leading `<kind>:` of a tool error.
- **The rename to noun-first names was a hard break with no aliases**, shipped
  in one release.

## Consequences

- A new agent capability is one spec and one `Mount` line. The catalog test
  fails when a spec is missing from `All()`, the desktop test fails when a
  server does not serve exactly its catalog tools, and the CLI parity test
  fails when an op has no command.
- `internal/ops` imports only `internal/domain`, its own packages and
  `platform/observe`, so both programs compile it without each other's
  dependencies (depguard `ops-is-a-contract`).
- A field flag must not share a name with a `hive dt` flag (`--caller`,
  `--desktop-url`, `--input`); `opscli` refuses one.
- A field whose type is a slice of structs, a map or a struct has no flag. The
  CLI sets it through `--input`.
- Engine operations, a stdio MCP server and desktop-UI parity build on the same
  specs and are later work.
