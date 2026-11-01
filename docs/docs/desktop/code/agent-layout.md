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
- a tmux window of a Code session by id (`@3`) or exact name.

## What an agent can do

| Tool | CLI | What it does |
| --- | --- | --- |
| `open_tab` | `hive dt tab open <view>` | Opens Drive, Diff, Tasks, or a terminal. `--path` picks a Drive document or canvas. `--placement behind` adds the tab behind the one in front with a dot; `beside` and `below` split the panel. `--anchor` names the tab to place next to. |
| `close_tab` | `hive dt tab close <tab>` | Closes a Drive, Diff, or Tasks tab. On a Code window it kills that tmux window. |
| `move_tab` | `hive dt tab move <tab> --placement P` | Moves an open tab behind, beside, or below the anchor, or the panel in front. |
| `focus_tab` | `hive dt tab focus <tab>` | Brings a tab forward and focuses its panel. This one takes the screen. |
| `close_panel` | `hive dt panel close <tab>` | Closes the panel holding the tab; its tabs join the neighbour. |
| `resize_split` | `hive dt split resize <tab> --share 0.6` | Gives the side holding the tab that share of its split, from 0.2 to 0.8. |
| `turn_split` | `hive dt split turn <tab>` | Turns the split between side by side and stacked. |
| `get_layout` | `hive dt layout` | Prints the panels, their tabs, and the splits as JSON. |

A new terminal opens detached, so it never takes the keyboard. It is a shell in the session's checkout with nothing running in it, and `--name` names the window. The tool takes no command: an agent runs its own work itself and opens a terminal for you to type in.

## From the CLI

The commands find the running app through a file the desktop writes in the hive data directory when it starts. Pass `--url http://127.0.0.1:<port>` or set `HIVE_DESKTOP_URL` to reach a different instance, such as a development build. `--session` defaults to `HIVE_AGENT_SESSION` inside a chat and to the current directory inside a Code session's checkout.

```bash
hive dt layout | jq
hive dt tab open terminal --name tests --placement beside
hive dt tab move drive --placement below --anchor tests
hive dt split resize tests --share 0.4
```

A refused request prints the reason, such as `invalid: a chat has only a Drive tab`, and exits with status 1.

## From MCP

The same tools are on the **Hive Drive** server, which a workspace enables under **MCP servers** and a Code session's agent adds from **Settings ▸ MCP servers**. See [Let an agent write](../chats/agent-workspaces.md#let-an-agent-write) for the document tools beside them.
