---
icon: lucide/list-checks
description: An overview of the core features in the Inbox, Code, and Chats areas.
---

# Features

Hive Desktop brings incoming work, coding sessions, and long-running agent workspaces into one app. Its three main areas can be used together or independently.

## Inbox

Inbox turns scattered updates into focused feeds. Connect the systems you already use, route each item through rules you control, and decide when Hive should notify you or start work.

<div class="grid cards hive-feature-grid" markdown>

-   :lucide-plug:{ .lg .middle } __Multiple sources__

    ---

    Collect work from GitHub, Gitea and Forgejo, Grafana, PostHog, RSS and Atom feeds, local webhooks, and commands that emit JSON.

-   :lucide-workflow:{ .lg .middle } __Visual routing__

    ---

    Connect sources to filters and JavaScript functions, then route matching items to feeds, notifications, and actions. A flow can branch to several destinations.

-   :lucide-inbox:{ .lg .middle } __Focused feeds__

    ---

    Organize feeds into folders, search and sort items, filter unread work, archive completed items, and restore ignored items from Trash.

-   :lucide-history:{ .lg .middle } __Item timeline__

    ---

    Inspect an item's content, source, noteworthy changes, available actions, and linked coding sessions in one detail view.

-   :lucide-zap:{ .lg .middle } __Actions__

    ---

    Start a coding session, run a shell command, or publish a message from an item or flow. Item actions can also copy rendered text.

-   :lucide-bell-ring:{ .lg .middle } __Change-aware notifications__

    ---

    Send one system banner or in-app notification when a routed item is new or changes. Read and archive state stays local to Hive.

</div>

See [How Hive works](../inbox/how-it-works.md), [Sources](../inbox/sources.md), [Flows](../inbox/flows.md), and [Actions](../inbox/actions.md).

## Code

Code is a desktop control surface for parallel coding-agent sessions. Each task gets an isolated checkout and persistent terminal, while Hive keeps the agent, tests, shell, git state, pull request, and task tree in view.

<div class="grid cards hive-feature-grid" markdown>

-   :lucide-box:{ .lg .middle } __Persistent coding sessions__

    ---

    Create repository sessions, start or stop their agents, and return to their tmux terminals after switching views or restarting Hive. Sessions are also available in the hive CLI when both use the same Hive data root, which is the default.

-   :lucide-settings-2:{ .lg .middle } __Shared session configuration__

    ---

    Use the hive CLI's config, commonly `~/.config/hive/config.yaml`, for repositories, agent profiles, clone strategies, setup commands, and starting windows.

-   :lucide-panels-top-left:{ .lg .middle } __Windows and panes__

    ---

    Create, rename, reorder, split, resize, zoom, search, and close tmux windows and panes from the app.

-   :lucide-terminal:{ .lg .middle } __Scratch terminals__

    ---

    Keep repository-free shells in a dedicated tmux session.

-   :lucide-git-pull-request:{ .lg .middle } __Git and pull request status__

    ---

    See the current branch, local changes, unpushed commits, and pull request review and check state. Open the checkout in your editor or file manager.

-   :lucide-square-terminal:{ .lg .middle } __Pop-up and quick terminals__

    ---

    Open a temporary shell from any area, or give tools such as `lazygit`, a test watcher, or `btop` their own command-palette entry and shortcut. Hiding the pop-up leaves its shell running.

-   :lucide-list-checks:{ .lg .middle } __Tasks__

    ---

    Inspect the same `hive hc` task tree used by coding agents, including epics, subtasks, blockers, comments, checkpoints, and linked sessions.

-   :lucide-pin:{ .lg .middle } __Pinned chats__

    ---

    Attach a persistent chat from Chats beside repository sessions without stopping or duplicating its agent.

</div>

See [Terminal mode](../code/terminal-mode.md), [Actions](../inbox/actions.md#quick-terminals), [Hive sessions](../../cli/getting-started/sessions.md), [Hive CLI configuration](../../cli/configuration/index.md), and [Hive task tracking](../../cli/getting-started/task-tracking.md).

## Chats

The Chats area gives ongoing agent work a durable home outside a repository session. Build a workspace around a purpose, give it the right instructions and tools, then use it interactively or run it on a schedule.

<div class="grid cards hive-feature-grid" markdown>

-   :lucide-messages-square:{ .lg .middle } __Persistent agent workspaces__

    ---

    Keep named chats, instructions, and generated agent configuration together. Chats continue when you switch areas or restart Hive.

-   :lucide-command:{ .lg .middle } __Flexible agent commands__

    ---

    Start from Claude Code or Codex presets, or provide a custom command for another CLI, wrapper, model, or permission setup.

-   :lucide-wrench:{ .lg .middle } __Skills and MCP servers__

    ---

    Choose reusable skill packages and tools for each workspace. Hive includes integrations for Hive Desktop, Hive Canvas, Playwright, and Chrome DevTools.

-   :lucide-calendar-clock:{ .lg .middle } __Scheduled jobs__

    ---

    Run a workspace prompt hourly, daily, weekly, monthly, or on a custom local-time cron schedule. Run it on demand, pause it, choose what happens after a missed run, and inspect recent outcomes.

-   :lucide-activity:{ .lg .middle } __Live agent state__

    ---

    See which chats are working, need approval, are running, or are stopped. Stop, restart, rename, or delete a chat independently of its workspace.

-   :lucide-panels-right-bottom:{ .lg .middle } __Canvases__

    ---

    Let agents publish durable Markdown, HTML, and links beside a chat or a Code session, then read them full page, search, copy, or export the result.

-   :lucide-panel-left-open:{ .lg .middle } __Code integration__

    ---

    Pin a chat into Code and use the same live tmux session from either area.

-   :fontawesome-brands-hive:{ .lg .middle } __Built-in Hive workspace__

    ---

    Ask an agent to configure feeds, flows, actions, settings, shortcuts, webhook sources, and other workspaces.

</div>

See [Agent workspaces](../chats/agent-workspaces.md).
