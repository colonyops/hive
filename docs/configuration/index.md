---
icon: lucide/settings
---

# Configuration

Config file: `~/.config/hive/config.yaml`

## Example

```yaml
repo_dirs:
  - ~/projects

agents:
  default: claude
  claude:
    command: claude
    flags: ["--model", "opus"]
  aider:
    command: /opt/bin/aider
    flags: ["--model", "sonnet"]

tmux:
  poll_interval: 1.5s
  preview_window_matcher: ["claude", "aider"]

tui:
  theme: tokyo-night

rules:
  - pattern: ""
    max_recycled: 5
    windows:
      - name: "{{ agentWindow }}"
        command: '{{ agentCommand }} {{ agentFlags }}'
        focus: true
      - name: shell
    commands:
      - hive ctx init
```

!!! tip
    Run `hive doctor` to validate your configuration and check that all dependencies (git, tmux, plugins) are correctly set up.

## General Settings

| Option                        | Type       | Default              | Description                                 |
| ----------------------------- | ---------- | -------------------- | ------------------------------------------- |
| `repo_dirs`                   | `[]string` | `[]`                 | Directories to scan for repositories        |
| `copy_command`                | `string`   | `pbcopy` (macOS)     | Command to copy to clipboard                |
| `auto_delete_corrupted`       | `bool`     | `true`               | Auto-delete corrupted sessions on prune     |
| `history.max_entries`         | `int`      | `100`                | Max command palette history entries         |
| `clone_strategy`              | `string`   | `"full"`             | Default clone strategy: `"full"` or `"worktree"` (see [Clone Strategies](#clone-strategies)) |

## Agents

Agent profiles define the AI tools available for spawning in sessions. The `default` key selects which profile to use when creating a new session.

| Option                  | Type       | Default        | Description                                          |
| ----------------------- | ---------- | -------------- | ---------------------------------------------------- |
| `agents.default`        | `string`   | `"claude"`     | Profile name to use by default                       |
| `agents.<name>.command` | `string`   | profile name   | CLI binary to run (defaults to profile name if empty)|
| `agents.<name>.flags`   | `[]string` | `[]`           | Extra CLI args appended to the command on spawn      |

Sessions can run multiple agents by opening additional tmux windows — use `tmux.preview_window_matcher` to control which windows the TUI monitors.

## Tmux

| Option                        | Type       | Default                             | Description                           |
| ----------------------------- | ---------- | ----------------------------------- | ------------------------------------- |
| `tmux.poll_interval`          | `duration` | `1.5s`                              | Status check frequency                |
| `tmux.preview_window_matcher` | `[]string` | `["claude", "aider", "codex", ...]` | Regex patterns for agent window names |

## TUI

| Option                 | Type       | Default        | Description                                  |
| ---------------------- | ---------- | -------------- | -------------------------------------------- |
| `tui.theme`            | `string`   | `tokyo-night`  | Built-in theme name (see [Themes](themes.md))|
| `tui.refresh_interval` | `duration` | `15s`          | Auto-refresh interval (0 to disable)         |
| `tui.preview_enabled`  | `bool`     | `true`         | Enable tmux pane preview sidebar on startup  |
| `tui.group_by`         | `string`   | `repo`         | Tree view grouping: `repo` or `group`        |

## Messaging

| Option                   | Type     | Default  | Description                  |
| ------------------------ | -------- | -------- | ---------------------------- |
| `messaging.topic_prefix` | `string` | `agent`  | Default prefix for topic IDs |
| `context.symlink_name`   | `string` | `.hive`  | Symlink name for context dir |

## Todos (Experimental)

| Option                               | Type                | Default | Description |
| ------------------------------------ | ------------------- | ------- | ----------- |
| `todos.actions`                      | `map[string]string` | `{}`    | Custom enter handlers for URI schemes |
| `todos.limiter.max_pending`          | `int`               | `0`     | Global pending-todo cap (`0` disables) |
| `todos.limiter.rate_limit_per_session` | `duration`        | `0`     | Per-session add cooldown (`0` disables) |
| `todos.notifications.toast`          | `bool`              | `true`  | Show toast on todo creation |

## Clone Strategies

Hive supports two strategies for isolating session repositories:

### `full` (default)

Each session gets a complete `git clone` of the remote. This is the simplest approach and works with any workflow.

```
~/.local/share/hive/repos/
└── myproject-abc123/          ← full clone (independent .git directory)
    ├── .git/
    └── src/
```

### `worktree`

All sessions for the same remote share a single bare clone (`repos/.bare/<owner>/<repo>/`). Each session is a [git worktree](https://git-scm.com/docs/git-worktree) pointing into the shared bare clone. This avoids re-downloading repository history for each session and makes fetches faster.

```
~/.local/share/hive/repos/
├── .bare/
│   └── acme/myproject/        ← shared bare clone (fetch target)
├── myproject-wt-abc123/       ← session worktree (.git is a file)
│   ├── .git                   ← gitdir: .../.bare/.../worktrees/...
│   └── src/
└── myproject-wt-def456/       ← another session worktree
    ├── .git
    └── src/
```

**When to use worktree:**

- Large repositories where cloning takes a long time
- Many parallel sessions for the same repository
- Teams or workflows requiring fast session recycling

**Set globally** in `config.yaml`:

```yaml
clone_strategy: worktree
```

**Override per rule** (see [Rules](rules.md#rule-fields)):

```yaml
rules:
  - pattern: ".*github.com/acme/.*"
    clone_strategy: worktree
```

**Override per session** with the CLI flag:

```bash
hive new --remote git@github.com:acme/myproject.git --clone-strategy worktree my-task
```

Priority: CLI flag > rule override > global `clone_strategy`.

## More Configuration

- **[Rules](rules.md)** — Repository-specific spawn, recycle, setup commands, and file copying
- **[User Commands](commands.md)** — Custom commands for the vim-style command palette
- **[Keybindings](keybindings.md)** — Map keys to user commands or built-in actions
- **[Todo Configuration (Experimental)](todos.md)** — Todo actions, limiter, notifications, and enter behavior
- **[Plugins](plugins.md)** — External service integrations (tmux, Claude, GitHub, Beads, etc.)
- **[Themes](themes.md)** — Built-in color palettes and custom theme creation

## Data Storage

All data is stored at `~/.local/share/hive/`:

```
~/.local/share/hive/
├── hive.db                    # SQLite database (sessions, messages)
├── bin/                       # Bundled scripts (auto-extracted)
│   ├── hive-tmux              # Tmux session launcher
│   └── agent-send             # Send text to agent in tmux
├── repos/                     # Cloned repositories
│   ├── myproject-feature1-abc123/      # full-clone session
│   ├── .bare/                          # worktree sessions share bare clones
│   │   └── owner/myproject/            # bare clone (shared fetch target)
│   ├── myproject-wt-abc123/            # worktree session
│   └── myproject-wt-def456/            # another worktree session
└── context/                   # Per-repo context directories
    ├── {owner}/{repo}/        # Linked via .hive symlink
    └── shared/                # Shared context
```
