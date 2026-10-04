# Hive Orchestrator

The `hive-orchestrator` MCP server gives a workspace's agent control over hive
sessions: the isolated repository checkouts the hive CLI and the Code area run
agents in. With it an agent can split work across repositories, start a
session per repository with a prompt, watch each one, answer the ones blocked
on a permission prompt, and wait for them to report back on the message bus.

Like `hive-desktop`, the server is the running app itself. It is a separate
entry because it is the most powerful thing a workspace can declare: starting a
session runs a command, and typing into an agent's pane is acting as the user.

## Authority

Declaring this server in a workspace's `mcps:` list is the grant. The server
checks it on every call: the request's bearer token must be the token Hive
handed one of this workspace's chats at launch (`HIVE_AGENT_SESSION_TOKEN`),
and that chat's workspace must list `hive-orchestrator`. A chat in another
workspace holds a token too, and is refused.

The generated config names the environment variable, never its value, so no
credential is written into the workspace folder. Claude Code expands
`${HIVE_AGENT_SESSION_TOKEN:-}` in the `Authorization` header; Codex reads it
through `bearer_token_env_var`. Opened outside Hive, the variable is unset and
every call is refused as `unauthenticated`.

A client outside a workspace uses an access token from Settings ▸
Orchestrator instead.

A workspace's agent can edit its own manifest, and any agent with a shell can
run `hive session send`. The declaration decides what the agent is offered; it
is not a sandbox.

## Launch

Streamable HTTP on the loopback server, at a URL resolved when the catalogue is
rendered:

```
http://127.0.0.1:<port>/mcp/orchestrator
```

## Stability

`experimental`. Tool names and shapes may change.

## What it needs

`http.enabled` in `settings.yaml`, which is on by default, and tmux. Sessions
the agent starts run the hive config's agent profiles, so an agent profile
that bypasses permissions runs unattended in every session started here.
