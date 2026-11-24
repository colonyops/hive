# Session operations route through the server fleet and a generated command keeps the spellings of the command it replaces

- **Status:** accepted
- **Date:** 2026-11-24

## Context

The desktop owned `session_list` and `session_tags_update`, and they read only this machine's sessions. The hand-written `hive session` commands reached every configured server. The orchestrator, Hive Desktop and agents script those commands: `--json`, `-r`/`-s`/`-a`/`--bg`, `HIVE_SERVER`, `<server>:<id>` and the hidden root `hive ls`. ADR generated-cli-commands-nest-each-word-of-an-operation-s-resource-as-a-group-below-a-namespace removed `--json` and short flags from generated commands, and when #880 replaced `hc`, `msg` and `todo` that way, scripts broke.

## Decision

**The session operations are engine operations whose handlers reach the server fleet, and their generated `hive session` commands keep every spelling of the hand-written ones.**

- `internal/ops/sessions` declares `session.list`, `get`, `update` and `session_tags.update` in the `sessions` toolset (`/mcp`), and `session.create`, `start`, `recycle` and `delete` in `session-control`. `recycle` and `start` join `ops.Verbs`.
- **Every session id is server-qualified**: `<server>:<id>`, or a bare id on `server` (default `local`). `engineops.WithFleet` gives the handlers the current `servers.Fleet`. A list reads every server at once; a server that fails goes to `offline` unless the list names it.
- **`session-control` is its own server**, `hive-sessions` at `/mcp/sessions`, with the token and bearer check of `hive-chats` (ADR chat-control-is-served-on-a-token-guarded-mcp-server-whose-token-the-endpoint-file-carries). Create and start run an agent command, recycle runs the recycle commands, and delete removes a checkout, on any repository and server, so they are a grant of their own (ADR a-workspace-declares-its-own-authority). `/mcp` has no token, so it cannot serve these. `hive mcp serve` gives a session-control call 10 minutes, not 10 seconds, because a clone or a risk check can take minutes.
- **A generated command keeps the spellings of the command it replaces.** `hive session` commands have a hidden `--json`, `--server` reads `HIVE_SERVER`, `session create` keeps `-r`, `-s`, `-a` and `--bg`, and the hidden root `hive ls` stays. The JSON lines are the ones the hand-written commands printed, plus new fields.
- **`hive session info` and `hive session attach` stay hand-written**: `info` reads the working directory's session, and `attach` takes over the terminal. `hive new` calls the create handler, then attaches.

## Consequences

- `session_list` and `session_tags_update` answer the CLI shape: `repo` is the repository name and `remote` its URL, keys are snake_case, and the tags update answers the session itself. Corrupted sessions are in `corrupted` unless `state` asks for them.
- `hive dt session list` and `hive dt session tags update` are gone; `hive session list` and `hive session tags update` replace them.
- Piped `hive ls` and `hive session list` print JSON Lines, not the table, and a failing command prints its error as one `{"kind","message"}` line on stderr instead of text. The message text stays, the `hive servers ls` hint for an unreachable server included, because `ops.Wrap` keeps the cause for the CLI. A risk refusal exits 4 and an unknown server exits 3, where both exited 1.
- A later replacement of hand-written commands must carry their flags as hidden spellings, with a test that runs each scripted form.
