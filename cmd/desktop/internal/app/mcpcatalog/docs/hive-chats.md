# Hive Chats

The `hive-chats` MCP server starts agent workspace chats and pins them to the
Code sidebar. It is how an orchestrator agent hands work to another workspace:
it starts a named chat with an opening prompt, and the user finds the chat in
Chats, or in the Code sidebar when it is pinned.

| Tool | What it does |
| --- | --- |
| `chat_list` | Every chat, or one workspace's, with `running` and `pinnedAt`. |
| `chat_create` | Start a chat: `workspace`, `name`, optional `prompt` and `pinToCode`. |
| `chat_pin_put` | Pin a chat to the Code sidebar. |
| `chat_pin_delete` | Unpin a chat. |

The same operations are `hive dt chat list`, `hive dt chat create`, and
`hive dt chat-pin put|delete`.

## Authority

`chat_create` runs the target workspace's command, with that workspace's MCP
servers and permission settings. A workspace whose command bypasses
permissions starts an agent that acts without asking. Grant `hive-chats` only
to a workspace you trust to start others.

A started chat is not a scheduled chat: Hive passes the prompt as written and
does not tell the agent to end the session.

## Launch

The server requires a bearer token
(ADR chat-control-is-served-on-a-token-guarded-mcp-server-whose-token-the-endpoint-file-carries).
The desktop mints the token on every start and writes it to the endpoint file
in the hive data directory, readable only by the user. `hive mcp serve` reads
it on every call, so a workspace's entry runs
`hive --data-dir <dir> mcp serve --toolset chats` over stdio.

The entry has no HTTP form. When the `hive` command does not report the same
version as the app, the catalogue reports a problem instead of an address.

## What it needs

`http.enabled` in `settings.yaml` (on by default), and the hive CLI at the
app's version on the session PATH, or at `daemon.hive_path`.
