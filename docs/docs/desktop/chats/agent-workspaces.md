---
icon: lucide/bot
description: Run persistent coding agents, scheduled jobs, skills, and MCP servers in named workspaces.
---

# Agent workspaces

The **Chats** area runs coding agents in persistent workspaces. Open it with <kbd>g</kbd> then <kbd>a</kbd>.

A workspace contains its instructions, chats, launch command, skill packages, MCP servers, schedules, and canvases. Chats run in tmux, so they can continue when you switch views or restart Hive.

## Requirements

- tmux 3.2 or newer;
- an agent CLI or custom launch command available on Hive's `PATH`.

Hive includes command presets for Claude Code and Codex. A custom command can start another agent CLI or wrap a preset with model flags, environment variables, or other tools. See [Troubleshooting](../getting-started/troubleshooting.md#a-coding-agent-is-not-found) if Hive cannot find a command.

## The Hive workspace

Hive creates a built-in workspace named **Hive**. Use it to configure flows, sources, actions, settings, shortcuts, webhook sources, and other workspaces.

Open the workspace and describe the change you want. Its shipped skills know the local config paths and the app's current schemas. Its default command asks before making changes.

## Create a workspace

Use **New workspace** in Chats. A workspace chooses:

- a name and directory;
- a launch command;
- skill packages;
- MCP servers;
- optional schedules.

The command picker includes **Ask**, **Auto**, and **Full** presets for Claude Code and Codex. **Custom** lets you edit the complete command template. Add model or provider flags to the command itself.

Schedules and feed-item launches need a command that passes the opening prompt
as a shell-quoted argument. Keep this suffix from the shipped presets:

```text
{{ if .Prompt }}-- {{ .Prompt | shq }}{{ end }}
```

Hive refuses a prompted launch if the custom command drops `.Prompt` or
interpolates it without `shq`.

**Full** presets bypass the agent's approval checks. Use them only when you trust the workspace instructions and every enabled tool.

## Work with chats

Create several named chats inside a workspace and use the sidebar to see whether each agent is working, needs approval, is running, or is stopped.

Stopping an agent keeps its chat record. Restarting a stopped chat resumes the conversation when its command and agent support it. You can also rename or delete a chat, filter the sidebar, and open recent chats from the command palette.

Drag image files or paste clipboard screenshots into a chat's terminal to give
the agent visual context. See [Image input](../code/terminal-mode.md#image-input)
for supported formats, limits, and saved clipboard images.

An Inbox item's **…** menu offers **Create Session** for Code and **Create Chat**
for Chats. Both open the same dialog on the requested target; use its compact
**Code** / **Chats** switch to change targets. A chat keeps the generated item
prompt, which you can edit before launch. A `launch-session` action with a fixed
`workspace` can do the same from a manual item action or a flow. See
[Actions](../inbox/actions.md#action-types).

## Scheduled jobs

Add a schedule from the workspace editor to start a chat automatically. Schedules can run hourly, daily, weekly, monthly, or from a five-field cron expression in your local time.

Each schedule has its own prompt and controls to:

- pause or resume future runs;
- run it immediately without changing the next scheduled time;
- launch once after Hive reopens or skip a run missed while the app was closed;
- preview upcoming times and inspect recent launched, skipped, or failed runs.

Scheduled chats run unattended. If the previous scheduled chat is still active, Hive skips the next occurrence instead of starting overlapping work. Ask the agent to save durable output to a canvas or workspace file.

## Workspace files

Workspaces live under `~/.config/hive/desktop/workspaces/` by default. **Settings ▸ Chats** shows the current root. To move it, set `agent_workspaces.dir` in `settings.yaml` and restart Hive.

Each workspace has an `agent-workspace.yaml` manifest and an `AGENTS.md` instruction file. Hive generates agent-specific configuration when the workspace opens. Put lasting instructions in `AGENTS.md` because generated files are replaced.

## Skills

A workspace selects skill packages, such as the built-in `hive` package. Packages can include shipped skills and custom skills from the shared workspace folder.

The built-in Hive package covers:

- flows and sources;
- actions and quick terminals;
- settings and keybindings;
- agent workspaces;
- the Hive Desktop MCP server.

## MCP servers

Hive includes these MCP entries:

- **Hive Desktop** for reading and configuring the running app. It requires the local HTTP server, which is enabled by default.
- **Hive Canvas** for durable Markdown, HTML, and links beside a chat.
- **Playwright** for browser automation. It requires Node.js and a Playwright browser.
- **Chrome DevTools** for a running Chrome browser. It requires Node.js 20.19 or newer and Chrome.

You can add HTTP, SSE, and stdio servers to the shared MCP library, then enable them per workspace. Secrets in custom server configuration can use `env:`, `file:`, or `op://` references.

## Canvases

An agent with **Hive Canvas** enabled can publish named output beside its chat. Canvases remain available after the chat ends and can contain Markdown, sanitized HTML, and links.

Use the canvas pane to search previous output, copy a canvas as Markdown, save it to a file, or open its links. Settings ▸ Chats changes text size and line spacing for every canvas. Copy and save keep the original Markdown.

## Use a chat in Code

Pin a chat to the Code session tree when you want it beside repository terminals. It attaches to the same tmux session from either area. Unpinning it does not stop the agent.
