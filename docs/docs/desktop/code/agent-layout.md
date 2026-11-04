---
icon: lucide/layout-panel-left
description: Let an agent open, move, and arrange the tabs beside its chat or Code session, from MCP or from the hive CLI.
---

# Agents and the layout

An agent can do with a session's tabs and panels what you can: open Drive, Diff, Tasks, or a terminal beside itself, move and focus tabs, split, resize, and close panels, and read the layout first. The controls reach the app in two ways that share one implementation: the **Hive Drive** MCP server, and the `hive dt` commands of the [hive CLI](../../cli/getting-started/index.md).

Every request applies only while you are viewing that chat or Code session, and nothing acknowledges it. A chat has its one terminal and a Drive tab; Diff, Tasks, and new terminals belong to a Code session.

## Name a tab

A tab is named by what it shows:

- `drive`, `diff`, or `tasks` for a view;
- `terminal` for a chat's terminal;
- `applet:<id>` for an applet's tab;
- a tmux window of a Code session by id (`@3`) or exact name.

## What an agent can do

| Tool | CLI | What it does |
| --- | --- | --- |
| `tab_open` | `hive dt tab open <view>` | Opens Drive, Diff, Tasks, a terminal, or `applet:<id>` for an [applet](../configuration/applets.md) that lists a tab. `--path` picks a Drive document or canvas. `--placement behind` adds the tab behind the one in front with a dot; `beside` and `below` split the panel. `--anchor` names the tab to place next to. |
| `tab_close` | `hive dt tab close <view>` | Closes a Drive, Diff, Tasks, or `applet:<id>` tab. `hive dt tab close terminal --window <window>` kills that tmux window of a Code session. |
| `tab_move` | `hive dt tab move <tab> --placement P` | Moves an open tab behind, beside, or below the anchor, or the panel in front. |
| `tab_focus` | `hive dt tab focus <tab>` | Brings a tab forward and focuses its panel. This one takes the screen. |
| `panel_close` | `hive dt panel close <tab>` | Closes the panel holding the tab; its tabs join the neighbour. |
| `split_resize` | `hive dt split resize <tab> --share 0.6` | Gives the side holding the tab that share of its split, from 0.2 to 0.8. |
| `split_turn` | `hive dt split turn <tab>` | Turns the split between side by side and stacked. |
| `layout_get` | `hive dt layout get` | Prints the panels, their tabs, and the splits as JSON. |

A new terminal opens detached, so it never takes the keyboard. It is a shell in the session's checkout with nothing running in it, and `--name` names the window. The tool takes no command: an agent runs its own work itself and opens a terminal for you to type in.

## From the CLI

The commands find the running app through a file the desktop writes in the hive data directory when it starts. Pass `--desktop-url http://127.0.0.1:<port>` or set `HIVE_DESKTOP_URL` to reach a different instance, such as a development build. `--caller` defaults to `HIVE_AGENT_SESSION`, which Hive sets in a chat and in every Code session's terminals, and to the current directory when that variable is not set. Every command takes its fields as flags, or the whole input as JSON with `--input` (inline, `@file`, or `-` for stdin).

```bash
hive dt layout get | jq
hive dt tab open terminal --name tests --placement beside
hive dt tab move drive --placement below --anchor tests
hive dt split resize tests --share 0.4
```

A refused request prints the reason, such as `invalid: a chat has only a Drive tab`, and exits with a status for its kind:

| Kind | Exit status |
| --- | --- |
| `invalid` | 2 |
| `not_found` | 3 |
| `conflict` | 4 |
| `unavailable` | 5 |
| `unauthenticated` | 6 |
| any other failure | 1 |

`hive dt` has a command for every tool on the Hive Desktop and Hive Drive servers, not only the layout tools: `hive dt document put`, `hive dt inbox list`, `hive dt schedule list`, and so on. Run `hive dt --help` for the list.

## From MCP

The same tools are on the **Hive Drive** server, which a workspace enables under **MCP servers** and a Code session's agent adds from **Settings ▸ MCP servers**. See [Let an agent write](../chats/agent-workspaces.md#let-an-agent-write) for the document tools beside them.

`hive mcp serve` serves the same tools over stdio, so an agent's configuration needs no port. It finds the running app the way `hive dt` does, fills `caller` itself, and answers `unavailable: Hive Desktop is not running` while the app is closed. `--toolset` picks the tools: `drive,layout` for Hive Drive, or `inbox,sessions,actions,diagnostics,flows,workspaces` for Hive Desktop.

```bash
claude mcp add --scope user hive-drive -- hive mcp serve --toolset drive,layout
```

When the `hive` command is the same version as the app, workspaces and **Settings ▸ MCP servers** write this form for you. Otherwise they write the HTTP address.
