---
icon: lucide/network
description: Let a workspace's agent split work across repositories into hive sessions and drive them.
---

# Orchestrator

An orchestrator is an agent workspace whose agent works above your coding sessions. Give it a task that spans repositories and it starts a hive session in each one with a prompt, watches them, answers the ones waiting on a permission prompt, and reports back. The sessions do the coding in their own checkouts, the same sessions the Code area and the [hive CLI](../../cli/getting-started/sessions.md) show.

## Set it up

Hive adds a workspace named **Orchestrator** the first time it starts with this feature. Its `AGENTS.md` explains how to split work, write a session prompt, and when to ask you. If you delete it, Hive does not add it again, and it never replaces a workspace directory you already have named `orchestrator`.

To turn another workspace into one, add **Hive Orchestrator** to its MCP servers, or add `hive-orchestrator` to the `mcps:` list in its `agent-workspace.yaml`. The workspace needs the local HTTP server, which is on by default, and tmux.

## What it can do

The **Hive Orchestrator** server gives the agent these tools:

| Tool | What it does |
| --- | --- |
| `list_repositories` | The repositories and agent profiles a session can start with. |
| `start_session` | Create a hive session in a repository and start its agent with a prompt. |
| `list_sessions` | The sessions it started, each with its agent's status. |
| `peek_session` | One read of a session's agent: its state, context use, and the end of its screen. |
| `send_prompt` | Type a prompt into a session's agent, press Enter, and return the screen. |
| `send_keys` | Press keys in a session, such as `Down` and `Enter` to answer a prompt, and return the screen. |
| `wait_for_session` | Wait until a session's status changes or reaches the states you name. |
| `publish_message`, `wait_for_message` | Send and receive on the hive message bus. |
| `sleep` | Wait before checking again. |

Every session it starts is tagged `orchestrator`. `hive session list --tags orchestrator` shows them from a terminal.

## Authority

Starting a session runs its agent, and typing into a session acts as you. Only a workspace that lists `hive-orchestrator` can use the server: each call carries the session token Hive gave the chat when it launched, and a chat from another workspace is refused. The generated MCP config names the token's environment variable, never the token itself.

## Use it from outside Hive

An agent or worker that does not run in a Hive workspace needs an access token:

1. Open **Settings ▸ Orchestrator** and choose **New token**.
2. Copy the token. Hive keeps only a hash of it and cannot show it again.
3. Set it as `HIVE_ORCHESTRATOR_TOKEN` where the client runs, and add the server with the config the dialog shows.

The server listens on this machine only. Revoke a token in the same pane to cut off whatever uses it. Each token keeps its own place on the message bus, so a worker and a workspace chat do not take each other's messages.

## Defaults

The **Orchestrator** workspace starts with the Claude Code **Ask** command, so the agent asks before each tool call. The sessions it starts run your hive [agent profiles](../../cli/configuration/index.md#agents) as configured, including any permission flags they set.
