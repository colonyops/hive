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

Startup failures stay in a copyable report with the failed command and available terminal output. If tmux terminates later, Hive keeps a termination notice even when exit details are unavailable. **Restart** starts a new process. It also recovers a session whose launch did not finish, for example after Hive quit during a start. **Reconnect** restores a lost connection to a session that may still be running. Reports survive switching sessions within the app, but not an app restart.

A setup command that exits successfully does not stop healthy panes. If every configured command finishes successfully and no terminal remains, the panel shows **Session completed**.

**Stop session** stops the tmux session and its processes while keeping the checkout and Hive session record. **Recycle** and **Delete** also change or remove the checkout.

For repository sessions, the optional status bar shows the branch, changes against the default branch including uncommitted work, whether there is uncommitted work, and unpushed commits. Select the line counts to [review the changes](#changes). Connected GitHub, Gitea, and Forgejo repositories also show pull request, review, and check state. Use the status bar to open the checkout in your editor or file manager. Actions that target `statusbar` add buttons of their own; see [Where actions run](../inbox/actions.md#where-actions-run).

### Tags

Tags are short words you put on a session, such as `auto` or `backend`. A session row with tags shows a tag icon; hover it to read them. The status bar above the terminal lists the attached session's tags. **Edit session…** in the session menu or the command palette changes a session's name and tags in one dialog; clicking the tags in the status bar opens it on the tags. Press <kbd>Enter</kbd> or type a comma to add a tag, and <kbd>Backspace</kbd> to remove the last one. Tags already used on other sessions are suggested.

Tags ignore case, so `Auto` and `auto` are the same tag, and a tag cannot contain a comma. The sidebar filter matches tags as well as names. The hive CLI reads and writes the same tags; see [Sessions](../../cli/getting-started/sessions.md).

### Views

A view is a saved filter over your sessions. Pick one from the **…** menu at the top of the sidebar. **All sessions** and **Running** are built in. Running replaces the old **Only running sessions** toggle. If you had that toggle on, Code opens on Running. **Reload sessions** is in the same menu.

The filter button beside the search box adds a condition on a tag, a repository, the run status, or the session's age. While a filter is active, a bar under the toolbar shows each condition as a chip, written like a GitHub search qualifier: `status:running`, `tag:bug,ui`, `age:>7d`. A leading `-` means "not", as in `-repo:acme/site`. Hover a chip to read it in words. The **…** menu marks the active view. Click a chip to change it, and click its × to remove it. Removing the last chip goes back to All sessions. With two or more chips, **and** between them keeps a session only when every chip matches it. Click it to switch to **or**. The × at the end of the bar goes back to All sessions.

The bottom of the filter menu says how many sessions the view hides. When you change a view's chips, a dot appears on the filter button and the menu offers **Save** and **Reset**. On a built-in view, **Save as new view…** asks for a name and makes a new view. **Manage views…** in the **…** menu opens a drawer where you rename, reorder, and delete the views you made. Pinned chats and scratch terminals always stay visible. The search box searches within the current view.

Saved views live in `settings.yaml` under `views.code`, so they follow your config to other machines. Which view is showing stays per machine.

## Scratch terminals

The **Terminals** section holds a scratch tmux session for shells that are not tied to a repository. Use its `+` button or <kbd>⌘T</kbd> to add tabs.

Scratch tabs can be selected, renamed, reordered, and closed. The session keeps running when Hive closes. **Stop terminal** stops every scratch tab.

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

### Images in output

Images that Pi draws with the Kitty graphics protocol show in Chats, Code
panes, and pop-up terminals. Hive Desktop sets `PI_IMAGE_PROTOCOL=kitty` on the
chats, sessions, agent windows, and pop-ups it starts. A session started from
the hive CLI does not get it: set `PI_IMAGE_PROTOCOL=kitty` in that session's environment
yourself.

When you reopen a session or the terminal resyncs, Hive narrows the window by
one column for a moment so Pi redraws its images. A program that does not
redraw on a resize shows its text without the image.

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

Switch between two layouts at the top of the view:

- **List** shows epics with their subtasks nested under them. Filter it by status with the tabs on the right of the toolbar.
- **Board** shows a column for every status (Open, In Progress, Done, Cancelled). Drag a card to another column to change its status. Click a card to open its details in a drawer over the columns. Press <kbd>Esc</kbd> or click outside the drawer to go back to the board.

Search by title or ID, pick a repository (type in the picker to find one), and inspect epics, subtasks, blockers, comments, checkpoints, and linked sessions. You can change task status, follow blocker links, and prune completed work from the Tasks toolbar. Cancelling a task, or closing an epic that still has open subtasks, asks you to confirm first.

Each list row has its own actions. Click the status icon to pick a status, or hover the row for **Mark done** (**Reopen** on a closed task) and a menu with every status, **Copy ID**, and **Delete**. Right-clicking a row opens the same menu.

Use <kbd>J</kbd> and <kbd>K</kbd> (or the arrow keys) to move the selection. On the board, <kbd>H</kbd> and <kbd>L</kbd> move between columns. In the list, they fold and unfold epics. <kbd>D</kbd> marks the selected task done, or reopens it if it is closed. <kbd>Y</kbd> copies the selected task's ID.

## Changes

**Changes** shows what an agent changed in the session on screen, without leaving Hive. Open it with <kbd>⌘⌥G</kbd> (Windows/Linux: <kbd>Ctrl+Alt+G</kbd>), the file button in the status bar, or the line counts beside the branch. It opens over the session and closes with <kbd>Esc</kbd>.

Every changed file's diff sits in one scroll, with the files listed beside it. **Branch** compares the working tree with the point where the branch left the default branch, so it shows everything the agent did, committed or not. **Uncommitted** compares it with `HEAD`. Only Uncommitted appears on the default branch, or when Hive can't find a merge base, as in a shallow clone. Both include untracked files.

| Key | Action |
| --- | --- |
| <kbd>j</kbd> / <kbd>k</kbd> | Next or previous file |
| <kbd>n</kbd> / <kbd>N</kbd> | Next or previous hunk |
| <kbd>v</kbd> | Mark the file viewed, which folds it |
| <kbd>o</kbd> | Fold or unfold the file |
| <kbd>s</kbd> | Switch between unified and side by side |
| <kbd>/</kbd> or <kbd>⌘F</kbd> | Search the diffs |
| <kbd>r</kbd> | Show or hide the comments panel |
| <kbd>⌘↵</kbd> | Send your comments to the agent |
| <kbd>Esc</kbd> | Close |

The view refreshes every 5 seconds while it's open and the window has focus. A viewed file comes back unviewed when it changes again. Viewed marks last until you quit Hive.

Drag either divider to resize the file list or the comments panel. Hive remembers the widths, and whether the panel is open.

### Search

Press <kbd>/</kbd> or <kbd>⌘F</kbd> to search. <kbd>Enter</kbd> and <kbd>Shift+Enter</kbd> step through the matches, and a match in a folded file unfolds it. Search ignores case and covers the diffs Hive has read so far, up to 200 files around the one on screen. When it hasn't read them all, the bar shows how many it covered. It stops counting at 2,000 matches.

### Comments

To comment, drag across line numbers, click the **+** beside a line, or select code with the mouse. Type in the box that opens and press <kbd>⌘↵</kbd>. A label button saves that label as the comment. Comments appear under their lines and as rows in a panel beside the diff.

The send button in the header, which shows how many comments you've written, pastes them into the session's agent and submits them, then clears them. If the session has one agent, it goes straight there, even mid-turn. With several agents, a dialog asks which one. An agent waiting on a prompt can't receive comments, because the Enter would answer the prompt. If a send fails, or no agent is running, your comments stay and an error offers **Copy comments**, which copies them as the text the agent would have received.

The panel keeps the last three reviews you sent from each session under **Sent**, in case the agent never acted on one. Copy one, or restore it to edit and send again.

Deleting a session deletes its comments and sent reviews, so the confirmation warns about any unsent comments, and a message afterward offers to copy them. Recycling warns too, but a recycled session keeps its comments unless the recycle deletes it.

Comments are kept on this machine, per session, until you send or delete them, and they survive quitting Hive. They point at line numbers, so a comment shows an amber dot when the agent edits the file after you commented. If the line has left the diff, or the comment was made under the other comparison, the comment shows only in the panel. Select its line label to jump to it, which also switches the comparison back.

Changes never writes to the checkout, so it's safe to use while an agent works in it. It can't stage or commit. These limits apply:

- A diff over 20,000 changed lines shows **Open in editor** instead. So does a very large file, and the list stops at 1,000 files.
- Binary files, symlinks, pipes, and nested repositories show a note instead of a diff.
- An unstaged `mv` shows as a delete plus an untracked file.
- Untracked files have no line counts, so the totals match the status bar.
- A repository with no commits shows git's error.

## Drive

An agent in a Code session can publish documents to Drive the same way a chat does. Hive does not change your agent's configuration, so add the server yourself. **Settings ▸ MCP servers** shows the address of the Hive Drive server and the command or config entry for Claude Code, Codex, and other agents. For Claude Code it is:

```bash
claude mcp add --scope user --transport http hive-drive http://127.0.0.1:<port>/mcp/drive
```

The address stays the same across restarts. An agent already set up with `/mcp/canvas` keeps working. The server needs the local HTTP server, which is enabled by default. An empty Drive pane in Code links to the same settings page.

Open the Drive pane beside a session with the right-panel button in the title bar. A session's documents belong to its repository, so every session of that repository shows the same ones, and they remain after the session is deleted. They are in `<root>/<owner>/<repo>/` under the hive context directory, which `hive drive init` links into a checkout as `.hive`, so a file you write to `.hive/plans/` shows in the pane within a second.

The pane, the Drive page, search, reorganizing, and the document tools work as they do in [Chats](../chats/agent-workspaces.md#drive). A scratch terminal has no Drive pane.

An agent outside any chat or Code session, such as one you start in a plain terminal, can still write documents: it passes `global` as its session, and every such agent shares the Shared space. It has no pane. Read it on the Drive page under **Shared**, or find a document by name in the command palette.

## Appearance

**Settings ▸ Terminal** controls the terminal font, size, weight, line height, letter spacing, visible windows, status bar, and session age badges. Age badges are on by default with a five-day threshold and appear once a Hive session reaches the configured number of whole days since creation. Scratch terminals and pinned chats do not get one.

<kbd>⌘+</kbd> and <kbd>⌘-</kbd> step the text size by 2px from a terminal without opening Settings, up to 64px, and <kbd>⌘0</kbd> puts it back to 13px. On Linux, use <kbd>Ctrl+Shift</kbd> with the same keys.

Enable **Show scrollbars** in **Settings ▸ Terminal** to drag through scrollback
in single-pane and zoomed Code terminals. It is off by default and applies to
open terminals immediately. Scrollbars stay hidden while split panes are visible;
wheel scrolling and **Scroll to bottom** remain available. Programs using an
alternate screen may have no scrollback.

## Shared tmux sizing

All clients attached to a tmux session share one grid size. If the terminal does not fill the available pane, another attached client may be setting the size.

Detach or resize the other client, or add this to `tmux.conf`:

```text
set -g window-size largest
```

## Current limits

- Hive loads up to 2,000 lines of scrollback.
- A large output burst can skip intermediate lines while the view resynchronizes.
