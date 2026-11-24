# Hive Sessions

The `hive-sessions` MCP server creates, starts, recycles and deletes hive
sessions, on this machine or on a server from the `servers:` list in the hive
config. It is how an orchestrator agent hands a repository task to a new
coding agent and cleans up after it.

| Tool | What it does |
| --- | --- |
| `session_create` | Create a session: `name`, optional `remote`, `source`, `server`, `agent` and `tags`. It starts in the background. |
| `session_start` | Start the agent of a session that has no running terminal. |
| `session_recycle` | Reset a full-clone session's checkout for reuse. Refuses on unsaved work unless `force`. |
| `session_delete` | Delete a session, its clone and its tmux session. Refuses on unsaved work unless `force`. |

The same operations are `hive session create`, `start`, `recycle` and
`delete`. The tools that read and edit a session record (`session_list`,
`session_get`, `session_update`, `session_tags_update`) are on
`hive-desktop`.

## Authority

`session_create` clones a repository and runs the agent command of the hive
config in it, on any configured server. `session_recycle` runs the recycle
commands, and `session_delete` removes the checkout. Grant `hive-sessions`
only to a workspace you trust to start agents and to remove their work.

## Launch

The server requires the same bearer token as `hive-chats`
(ADR chat-control-is-served-on-a-token-guarded-mcp-server-whose-token-the-endpoint-file-carries).
`hive mcp serve` reads it from the endpoint file on every call, so a
workspace's entry runs `hive --data-dir <dir> mcp serve --toolset session-control`
over stdio.

The entry has no HTTP form. When the `hive` command does not report the same
version as the app, the catalogue reports a problem instead of an address.

## What it needs

`http.enabled` in `settings.yaml` (on by default), and the hive CLI at the
app's version on the session PATH, or at `daemon.hive_path`.
