---
icon: lucide/square-terminal
description: View and control Hive tmux sessions in the Code area.
---

# Terminal mode

Open **Code** with <kbd>g</kbd> then <kbd>c</kbd>. Select a session to attach to its tmux windows.

## Requirements

Hive needs tmux 3.2 or newer for Code, Chats, and quick terminals.

```sh
brew install tmux
```

Hive searches your login shell's `PATH` and common package manager locations. Set `paths.tmux` in [Settings](../configuration/settings.md#advanced-configuration) if the binary is elsewhere.

The local HTTP server must also be enabled. It is on by default.

## Hive CLI compatibility

Code uses the same session records, isolated checkouts, and tmux sessions as the [hive CLI](../../cli/getting-started/index.md) when both use the same Hive data root. This is the default. Create, start, recycle, or delete a session from either interface and the other sees the same result.

Use the hive CLI config, commonly `~/.config/hive/config.yaml`, to configure the shared session engine:

- repository search paths and agent profiles;
- repository rules, clone strategies, and recycled checkouts;
- setup commands and the tmux windows or panes created for a new session.

Hive Desktop reads that file when it starts, so restart the app after changing it. Desktop-only settings such as terminal appearance, shortcuts, quick terminals, Inbox flows, and Chats workspaces remain under the Desktop config root, commonly `~/.config/hive/desktop/`.

See the hive CLI documentation for [sessions](../../cli/getting-started/sessions.md), the [configuration reference](../../cli/configuration/index.md), and [repository rules](../../cli/configuration/rules.md).

## Sessions

Code lists active Hive sessions and their tmux windows. Closing Hive leaves those sessions running. You can attach to the same session from another terminal with `tmux attach`.

If a session has no running terminal, select **Start session**. Hive creates its configured windows and starts the agent. This starts a new agent process and does not resume the previous terminal process.

**Kill terminal** stops the tmux session and its processes while keeping the checkout and Hive session record. **Recycle** and **Delete** also change or remove the checkout.

For repository sessions, the optional status bar shows the branch, changes against the default branch, uncommitted work, and unpushed commits. Connected GitHub, Gitea, and Forgejo repositories also show pull request, review, and check state. Use the status bar to open the checkout in your editor or file manager.

## Scratch terminals

The **Terminals** section holds a scratch tmux session for shells that are not tied to a repository. Use its `+` button or <kbd>⌘T</kbd> to add tabs.

Scratch tabs can be selected, renamed, reordered, and closed. The session keeps running when Hive closes. **Kill terminal** stops every scratch tab.

## Pop-up terminal

Press <kbd>⌘`</kbd> on macOS or <kbd>Ctrl+`</kbd> on Linux to open a shell over any area. In Code, it starts in the active pane's directory. Elsewhere, it starts in your home directory.

Hide the panel to keep its shell running, or use **End this terminal** to stop it. Quick terminal launchers use the same panel for tools such as `lazygit`, test watchers, and process monitors. See [Actions](../inbox/actions.md#quick-terminals).

## Working in Code

### Image input

Drag image files onto the terminal pane, or paste a screenshot from your
clipboard with <kbd>⌘V</kbd> on macOS. This works in Code, Chats, and pop-up
terminals. Hive inserts an image reference; add your instructions and submit
the prompt yourself. The agent must support image input and run on the same
filesystem as Hive.

PNG, JPEG, GIF, and WebP files are supported, up to 20 MiB and 64 megapixels
each. A gesture can include up to 10 images totaling 64 MiB. Unsupported or
oversized images show an error. Switching away during preparation cancels
insertion; a paste already sent can finish in its original pane.

Clipboard images are saved under `desktop/assets/terminal-images/` inside the
data directory shown in **Settings ▸ System**. They survive restarts so resumed
conversations can still read them. Storage is limited to 1 GiB. Remove files
you no longer need from that directory to free space; conversations that still
reference removed files cannot read them again. Dragged files stay at their
original location.

### Navigation

- Select a window from the sidebar to attach to it.
- Use <kbd>⌘1</kbd> through <kbd>⌘9</kbd> to switch windows.
- Use <kbd>⌘←</kbd> to focus the sidebar and <kbd>⌘→</kbd> to return to the terminal.
- Use <kbd>⌘B</kbd> (Windows/Linux: <kbd>Ctrl+B</kbd>) to show or hide the left sidebar, including while a terminal has focus.
- Press <kbd>/</kbd> while the sidebar is focused to filter sessions.
- Press <kbd>⌘F</kbd> in a terminal to search its recent scrollback.
- Drag windows in the sidebar to reorder them.

Press <kbd>?</kbd> for the current shortcut list. Shortcuts can be changed under **Settings ▸ Keyboard**.

Actions can add commands to session and window menus. Quick terminals can open tools such as `lazygit` in the active checkout. See [Actions](../inbox/actions.md).

## Panes

A window shows every tmux pane where tmux lays it out, including panes split from another tmux client. The pane with the keyboard is tmux's active pane, so a click in a pane also selects it in tmux.

- <kbd>⌘D</kbd> splits the active pane to the right and <kbd>⌘⇧D</kbd> splits it downward. The new pane opens in the active pane's directory.
- <kbd>⌘⌥←</kbd>, <kbd>⌘⌥→</kbd>, <kbd>⌘⌥↑</kbd>, and <kbd>⌘⌥↓</kbd> move between panes.
- Drag the border between two panes to resize them.
- <kbd>⌘⇧↩</kbd> zooms the active pane to fill the window, and again to restore the layout. A `zoomed` badge shows while a pane is zoomed.
- <kbd>⌘⇧W</kbd> closes the active pane. Closing the last pane closes the window. Hive asks first when the pane is running something.

On Linux, <kbd>Ctrl+Shift+D</kbd> splits right, <kbd>Ctrl+Shift+O</kbd> splits down, <kbd>Ctrl+Alt</kbd> with the arrow keys moves between panes, <kbd>Ctrl+Shift+M</kbd> zooms, and <kbd>Ctrl+Shift+Q</kbd> closes the pane.

## Tasks

Open **Tasks** from the Code status bar, the command palette, or its configured shortcut. It reads the same `hive hc` task tree used by coding agents and the CLI. See the hive CLI [task tracking guide](../../cli/getting-started/task-tracking.md) for the shared task model.

Filter by status or repository, search by title or ID, and inspect epics, subtasks, blockers, comments, checkpoints, and linked sessions. You can change task status, follow blocker links, and prune completed work from the Tasks toolbar.

## Canvases

An agent in a Code session can publish canvases the same way a chat does. Hive does not change your agent's configuration, so add the server yourself. **Settings ▸ MCP servers** shows the address of the Hive Canvas server and the command or config entry for Claude Code, Codex, and other agents. For Claude Code it is:

```bash
claude mcp add --scope user --transport http hive-canvas http://127.0.0.1:<port>/mcp/canvas
```

The address stays the same across restarts. The server needs the local HTTP server, which is enabled by default. An empty canvas pane in Code links to the same settings page.

Open the canvas pane beside a session with the right-panel button in the title bar. A session's canvases belong to its repository, so every session of that repository shows the same ones, and they remain after the session is deleted. They are stored under `canvases/` in the repository's hive context directory, which `hive ctx init` links into a checkout as `.hive`, and never in the checkout itself.

The pane, the full-page view, copy, and save work as they do in [Chats](../chats/agent-workspaces.md#canvases). A scratch terminal has no canvases.

An agent outside any chat or Code session, such as one you start in a plain terminal, can still write canvases: it passes `global` as its session, and every such agent shares one set of global canvases. They have no pane. Read them in the full-page view by picking **Global** in its owner picker, or find one by name in the command palette.

## Appearance

**Settings ▸ Terminal** controls the terminal font, size, weight, line height, letter spacing, visible windows, and status bar. <kbd>⌘+</kbd> and <kbd>⌘-</kbd> step the text size by 2px from a terminal without opening Settings, up to 64px, and <kbd>⌘0</kbd> puts it back to 13px. On Linux, use <kbd>Ctrl+Shift</kbd> with the same keys.

## Shared tmux sizing

All clients attached to a tmux session share one grid size. If the terminal does not fill the available pane, another attached client may be setting the size.

Detach or resize the other client, or add this to `tmux.conf`:

```text
set -g window-size largest
```

## Current limits

- Hive loads up to 2,000 lines of scrollback.
- A large output burst can skip intermediate lines while the view resynchronizes.
