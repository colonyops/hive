---
icon: lucide/bot
description: Run persistent coding agents, scheduled jobs, skills, and MCP servers in named workspaces.
---

# Agent workspaces

The **Chats** area runs coding agents in persistent workspaces. Open it with <kbd>g</kbd> then <kbd>a</kbd>.

A workspace contains its instructions, chats, launch command, skill packages, MCP servers, schedules, and a Drive space for the documents agents write. Chats run in tmux, so they can continue when you switch views or restart Hive.

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

Schedules, feed-item launches, and chats started with a prompt need a command
that passes the opening prompt as a shell-quoted argument. Keep this suffix from the shipped presets:

```text
{{ if .Prompt }}-- {{ .Prompt | shq }}{{ end }}
```

Hive refuses a prompted launch if the custom command drops `.Prompt` or
interpolates it without `shq`. For prompts larger than 32 KiB, `.Prompt` is a
short instruction to read and delete a private temporary file containing the
full text.

**Full** presets bypass the agent's approval checks. Use them only when you trust the workspace instructions and every enabled tool.

## Work with chats

Create several named chats inside a workspace and use the sidebar to see whether each agent is working, needs approval, is running, or is stopped.

Stopping an agent keeps its chat record. Restarting a stopped chat resumes the conversation when its command and agent support it. You can also rename or delete a chat, filter the sidebar, and open recent chats from the command palette.

Drag image files or paste clipboard screenshots into a chat's terminal to give
the agent visual context. See [Image input](../code/terminal-mode.md#image-input)
for supported formats, limits, and saved clipboard images.

### Tabs and splits

An open chat shows as a tab above its terminal. The `+` on the tab row opens the workspace's **Drive** tab beside it, and the two buttons at the right of the tab row split the panel to the right or downward, so a document can sit beside the conversation. Splits nest, tabs drag between panels, and the divider between two panels drags to resize. Hive remembers each chat's tabs and splits across restarts, and forgets them when you stop or delete the chat. Closing the chat's own tab stops the chat, as **Stop** on its row does.

A chat has one terminal and no Diff or Tasks tab; those belong to a [Code session](../code/terminal-mode.md#tabs-and-splits).

Images that Pi draws show in the chat's terminal. Hive sets
`PI_IMAGE_PROTOCOL=kitty` for chats it starts. See
[Images in output](../code/terminal-mode.md#images-in-output).

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

A skill that only one workspace needs does not have to go in the shared folder. Write it, or ask the workspace's agent to write it, into the workspace's `.claude/skills/<name>/SKILL.md` (or `.agents/skills/` for agents that read that folder). Hive keeps it when the workspace opens and only replaces skills that its packages install. A package skill with the same name overwrites it.

The built-in Hive package covers:

- flows and sources;
- actions and quick terminals;
- settings and keybindings;
- agent workspaces;
- the Hive Desktop MCP server.

## MCP servers

Hive includes these MCP entries:

- **Hive Desktop** for reading and configuring the running app. It requires the local HTTP server, which is enabled by default. Its `list_sessions` and `edit_session_tags` tools let an agent find hive sessions by tag and add or remove tags, so an orchestrating agent can pick up the sessions taged `auto` and retag each one it takes.
- **Hive Drive** for durable Markdown, HTML, and canvas documents beside a chat.
- **Playwright** for browser automation. It requires Node.js and a Playwright browser.
- **Chrome DevTools** for a running Chrome browser. It requires Node.js 20.19 or newer and Chrome.

You can add HTTP, SSE, and stdio servers to the shared MCP library, then enable them per workspace. Secrets in custom server configuration can use `env:`, `file:`, or `op://` references.

## Drive

Drive is the folder tree of documents that agents and you share. It holds Markdown, HTML, and canvas documents in folders, and it outlives any chat. The [hive CLI](../../cli/getting-started/context.md) reads the same tree.

### Where it shows

- **The tab.** The `+` on a chat's tab row opens a **Drive** tab beside it, with the workspace's folder tree and a reader for the selected document. Split the panel to read a document next to the conversation. A document an agent opens arrives as a Drive tab behind the one you are reading, with a dot until you look at it. See [Tabs and splits](#tabs-and-splits).
- **The Drive page.** Open it from the views menu (**Drive**), the command palette, or <kbd>⌘⇧P</kbd> (Windows/Linux: <kbd>Ctrl+Shift+P</kbd>) or <kbd>g</kbd> then <kbd>v</kbd>. It takes the whole window, like Settings, and opens on the space of the chat or Code session on screen. Its picker groups spaces as **Repositories**, **Workspaces**, and **Shared**. Press <kbd>Esc</kbd>, <kbd>⌘⇧P</kbd> again, or the close button to return to the view you came from. The page is a place in history, so Back returns to it, and a document on it has a link you can open again.

Search in a Drive tab or on the page looks across every space. Copy a document as Markdown, save it to a file, or open its links from the reader. A Markdown link to another document in the space opens it in place.

### Document kinds

- **Markdown** (`.md`) renders with its front matter shown above it.
- **HTML** (`.html`) runs in a sandbox. Scripts run inside the frame. The page cannot call Hive or send requests to localhost. It can load `https:` content.
- **Canvas** (`.canvas.json`) is a document of blocks that an agent builds: Markdown, Mermaid diagrams, sanitized HTML, and links.

Use a standalone `mermaid` fence inside a Markdown block for a diagram. Mermaid diagrams follow the current theme. Drag a diagram to pan, use its controls to zoom or fit it, or hold Control/Command while scrolling to zoom around the pointer. An invalid diagram shows an error in place without hiding the rest of the canvas.

A Markdown document or a canvas can include front matter such as tags, status, or an owner. Hive displays it above the document. A canvas also shows automatic creation and update times. Collapse **Properties** to hide it; Hive uses that choice for every document. Agents replace a canvas's editable front matter with the `set_frontmatter` tool; `created_at` and `updated_at` are reserved and updated by Hive. Values can be text, numbers, booleans, empty values, or lists of those values. Copy and save include the metadata as YAML front matter and preserve Mermaid source in a fenced `mermaid` block.

**Settings ▸ Drive** changes text size, line spacing, and text width for every document, in Chats and in Code. The text runs at a narrow reading width by default. **Wide** fits HTML laid out in columns, and **Full** runs to the edges of the view.

### Comment on a document

Comments live on the Drive page. A Drive tab beside a chat or a Code session does not show them; open the Drive page on the document to review it. Reviewing is off until you turn it on with the comment button in the document's header. Hive remembers the choice, and while it is off documents take no comments and show none. Select text in a Markdown document and a comment box floats under it with the selection marked; drag it by its top row to move it out of the way. To comment on a whole paragraph, list item, heading, code block, or table, point at it and click the comment button beside it in the margin. While nothing else has focus, the keyboard goes to the box:

- Start typing to write a comment, then press <kbd>⌘↵</kbd> or **Add Comment**.
- Press <kbd>1</kbd> to <kbd>4</kbd> to apply a label: **Clarify this**, **Verify this**, **Needs tests**, or **Looks good**. The icons in the box are the same labels. Text you typed first follows the label. Change them under **Settings ▸ Drive ▸ Comment labels**, or with `review.labels` in [settings](../configuration/settings.md).
- Press <kbd>Esc</kbd> to close the box.

A comment is anchored to the text you selected and the lines it is on, and that text is highlighted. Comments sit in a column beside the document, each on the row of its text; drag the column's edge to resize it. The title bar's right button shows or hides the column, and so does **Toggle comments** in the command palette, or <kbd>⌘⇧M</kbd>. Select a comment to scroll to its text, or highlighted text to find its comment; every comment has **Edit** and **Delete**. Write a note for the agent at the bottom of the column.

- **Copy All** puts the feedback on the clipboard: the document's path, then a numbered entry for the note and for each comment in document order, with its lines and the text it is about. The comments stay.
- **Send to Agent** (<kbd>⌘↵</kbd> in the note) opens a list of the agents running now, in Code sessions and chats, with their status. Sending pastes the feedback into the agent as one message, submits it, and clears the comments. An agent that is working gets it mid-turn; an agent waiting on a prompt cannot be picked until the prompt is answered.
- **Clear**, the trash button in the comments header, deletes the comments and the note after asking.

Comments are kept in Hive's database, not in the file, and survive a restart. If the document changes after you comment, the panel says the comments are on an earlier version. Send, copy, or discard them before adding new ones. Renaming or moving a document leaves its comments behind.

### Reorganize

You can reorganize the tree without an agent running. Create a folder, rename a document or folder, move it into another folder in the same space, or delete it from a row's menu. You can also drag a document onto a folder. Delete works on a document or an empty folder, and asks for confirmation. Deleting a document removes it for every chat or Code session that shares the space.

### Files on disk

Drive is a folder on disk under the hive context root, `~/.local/share/hive/context/` unless `context.base_dir` moves it:

| Space | Folder |
| --- | --- |
| A repository | `<root>/<owner>/<repo>/` |
| A Chats workspace | `<root>/workspaces/<name>/` |
| Shared | `<root>/shared/` |

A [Code session's](../code/terminal-mode.md#drive) checkout links its space as `.hive`. Drive refreshes within a second when a file changes on disk, so a document you write from a terminal or edit in Obsidian appears without a reload. Deleting a workspace or a chat leaves its documents in place.

### Let an agent write

An agent with **Hive Drive** enabled can publish documents beside its chat. The seeded Hive workspace has it on. The MCP server is at `/mcp/drive`; the older address `/mcp/canvas` answers the same server, so existing agent setups keep working. The six document tools are:

- `list_documents`
- `read_document`
- `write_document`
- `move_document`
- `delete_document`
- `open_document`

The ten canvas tools remain. A canvas name can include folders, so `reports/perf` is filed as `reports/perf.canvas.json`, and a bare name is filed under `canvases/`. An agent can only write `.md` and `.html` documents with `write_document`. `read_document` reads markdown, html, and any other text file in the space.

A repository with no documents yet links to the setup in **Settings ▸ MCP servers**.

## Use a chat in Code

Pin a chat to the Code session tree when you want it beside repository terminals. Use **Pin to Code** on a chat's row menu, or check **Pin to Code** in the New chat dialog to pin it when it starts. It attaches to the same tmux session from either area. Unpinning it does not stop the agent.
