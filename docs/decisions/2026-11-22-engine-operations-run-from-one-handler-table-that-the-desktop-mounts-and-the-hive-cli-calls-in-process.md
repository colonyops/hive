# Engine operations run from one handler table that the desktop mounts and the hive CLI calls in-process

- **Status:** accepted
- **Date:** 2026-11-22

## Context

ADR every-operation-is-declared-once-as-a-spec-in-internal-ops-and-each-surface-is-built-from-it left engine operations for later. `hive hc`, `hive msg` and `hive todo` called the engine from hand-written commands, and no MCP tool reached them, so an agent in a chat could read the inbox but could not claim a task. `internal/ops` may not import the engine (depguard `ops-is-a-contract`), so a spec cannot carry its handler.

## Decision

**An engine operation is a spec with `Owner: ops.OwnerEngine` in `internal/ops/<toolset>`, and `internal/hive/engineops` binds every one to a handler over `*hive.Engine`.** `Handlers.Ops()` is the one table: each entry mounts its tool (`mcpops.Mount`) and runs in-process from decoded arguments. The toolsets are `tasks`, `messages` and `todos`, served on `/mcp`.

- **The desktop mounts the table** on the server its toolsets name. **The hive CLI generates `hive <resource> <verb>` at the root** from the same specs (`hive task next`), because the command needs no running app. `hive dt` stays the namespace for desktop operations. A generated group whose name a hand-written group already holds (`todo`) gets the generated verbs added to it, and a verb both define is a startup panic.
- **The CLI prints JSON Lines.** An answer that implements `JSONLines() []any` prints one line per value, so a list prints one element per line; any other answer prints as one line.
- **Outputs keep the JSON the hand-written commands printed** (`hc.Item`, `hc.Comment`, `messaging.Message`, the todo fields), snake_case included. Inputs follow the catalog's camelCase names. One output shape on every surface lets the hand-written commands call the handlers without changing a line agents parse.
- **Caller.** A session caller acts as the session and a chat caller as its chat id, on either caller rule. On a session-or-global operation, a path inside no session is global with that directory, because the CLI sends its working directory when nothing names a caller; an id that names nothing stays `not_found`. Repo scope is `repo`, else the caller session's remote, else the git remote of the caller's directory. An `epicId` already scopes a read, so only an explicit `repo` narrows it, and a task a chat creates is still handed out to a session in a repository. `task_clear` with no repository refuses unless `allRepos` says to clear every one.
- **`next` joins `ops.Verbs`** for `task_next`.
- **The hand-written `hc`, `msg` and `todo` commands stay** as the older spelling. They keep their flags, their output and most of their error text, and call the handlers. Streaming (`msg sub --listen`) and payloads from stdin or a file stay on them only.

## Consequences

- A new engine operation is one spec and one `bind` line in `Handlers.Ops()`. Tests fail when `Ops()` and the engine ops of `catalog.All()` differ, when `/mcp` does not serve them, and when one has no command.
- `hive mcp serve` forwards engine operations to the desktop until it runs them in-process (#796). A forwarded call with `wait` gets that many seconds over the 10s timeout.
- Some refusals on the hand-written path now carry the kind of the operation (`todo update` of an unknown id exits 3, a blocker cycle exits 4) where they exited 1.
- Deleting the hand-written commands is later work, once agents use the generated ones.
