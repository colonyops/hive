# Engine operations run from one handler table that the desktop mounts and the hive CLI calls in-process

- **Status:** accepted
- **Date:** 2026-11-22

## Context

ADR every-operation-is-declared-once-as-a-spec-in-internal-ops-and-each-surface-is-built-from-it left engine operations for later. `hive hc`, `hive msg` and `hive todo` called the engine from hand-written commands, and no MCP tool reached them, so an agent in a chat could read the inbox but could not claim a task. `internal/ops` may not import the engine (depguard `ops-is-a-contract`), so a spec cannot carry its handler.

## Decision

**An engine operation is a spec with `Owner: ops.OwnerEngine` in `internal/ops/<toolset>`, and `internal/hive/engineops` binds every one to a handler over `*hive.Engine`.** `Handlers.Ops()` is the one table: each entry mounts its tool (`mcpops.Mount`) and runs in-process from decoded arguments. The toolsets are `tasks`, `messages` and `todos`, served on `/mcp`.

- **The desktop mounts the table** on the server its toolsets name. **The hive CLI generates the commands of the existing `hc`, `msg` and `todo` groups** from the same specs, because the command needs no running app. A table in `cmd/hive` maps each resource to its group (`task` to `hc`, `message` to `msg`, `topic` to `msg topic`, `todo` to `todo`), and each further word of a resource name is a nested group, so `task_comment.create` is `hive hc comment create`. `hive dt` stays the namespace for desktop operations and nests the same way (`chat_pin.put` is `hive dt chat pin put`). A verb that a hand-written command already holds is a startup panic.
- **The CLI prints JSON Lines** when stdout is not a terminal. An answer that implements `JSONLines() []any` prints one line per value, so a list prints one element per line; any other answer prints as one line. At a terminal, `hc list`, `hc get` and `hc context get` print a tree, a styled item and markdown.
- **Outputs keep the JSON the hand-written commands printed** (`hc.Item`, `hc.Comment`, `messaging.Message`, the todo fields), snake_case included. Inputs follow the catalog's camelCase names. One output shape on every surface keeps the lines agents already parse.
- **Caller.** A session caller acts as the session and a chat caller as its chat id, on either caller rule. On a session-or-global operation, a path inside no session is global with that directory, because the CLI sends its working directory when nothing names a caller; an id that names nothing stays `not_found`. Repo scope is `repo`, else the caller session's remote, else the git remote of the caller's directory. An `epicId` already scopes a read, so only an explicit `repo` narrows it, and a task a chat creates is still handed out to a session in a repository. `task_clear` with no repository refuses unless `allRepos` says to clear every one.
- **`next` joins `ops.Verbs`** for `task_next`.
- **The generated commands replace the hand-written ones** in `hc`, `msg` and `todo`. Their flags follow the operation's input fields, and the old verbs stay only as aliases (`hc show`, `hc ls`, `hc prune`, `msg pub`, `todo add`). The hand-written `msg sub` and `msg inbox` stay, because `--listen` and `--ack` have no operation.

## Consequences

- A new engine operation is one spec and one `bind` line in `Handlers.Ops()`. Tests fail when `Ops()` and the engine ops of `catalog.All()` differ, when `/mcp` does not serve them, and when one has no command.
- `hive mcp serve` forwards engine operations to the desktop until it runs them in-process (#796). A forwarded call with `wait` gets that many seconds over the 10s timeout.
- The rename breaks scripts that used the old flags (`hc create --parent`, `hc list --all`, `msg pub -t`, `todo add -u`). Refusals carry the kind of the operation (`hc next` with no task exits 3, a blocker cycle exits 4) where they exited 1.
