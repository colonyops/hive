---
icon: lucide/settings
---

# Configuration

Config file: `~/.config/hive/config.yaml`

## Example

```yaml
workspaces:
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

    Run `hive config` to dump the fully resolved configuration as JSON — useful for debugging which defaults and overrides are in effect.

## General Settings

| Option                        | Type       | Default              | Description                                 |
| ----------------------------- | ---------- | -------------------- | ------------------------------------------- |
| `workspaces`                  | `[]string` | `[]`                 | Directories to scan for repositories        |
| `copy_command`                | `string`   | `pbcopy` (macOS)     | Command to copy to clipboard                |
| `auto_delete_corrupted`       | `bool`     | `true`               | Auto-delete corrupted sessions on prune     |
| `history.max_entries`         | `int`      | `100`                | Max command palette history entries         |

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

| Option              | Type     | Default        | Description                                  |
| ------------------- | -------- | -------------- | -------------------------------------------- |
| `tui.theme`         | `string` | `tokyo-night`  | Built-in theme name (see [Themes](themes.md))|
| `tui.update_checker`| `bool`   | `true`         | Check for updates on startup                 |
| `tui.store`         | `bool`   | `false`        | Enable KV store browser tab                  |

## Messaging

| Option                   | Type     | Default  | Description                  |
| ------------------------ | -------- | -------- | ---------------------------- |
| `messaging.topic_prefix` | `string` | `agent`  | Default prefix for topic IDs |

## Context

| Option                 | Type     | Default                          | Description                                                                          |
| ---------------------- | -------- | -------------------------------- | ------------------------------------------------------------------------------------ |
| `context.symlink_name` | `string` | `.hive`                          | Symlink name created by `hive ctx init`                                              |
| `context.base_dir`     | `string` | `$HIVE_DATA_DIR/context/`        | Override the base directory for all context storage. Accepts `~` and absolute paths. |

By default context documents are stored under hive's data directory (`~/.local/share/hive/context/`). Set `context.base_dir` to redirect them elsewhere — for example, into a git repository so plans and research are version-controlled alongside your code.

```yaml
context:
  base_dir: ~/notes/hive-context   # store in a dedicated git repo
```

See [Git-backed Context](../recipes/git-backed-context.md) for a practical guide.

## Todos (Experimental)

| Option                               | Type                | Default | Description |
| ------------------------------------ | ------------------- | ------- | ----------- |
| `todos.actions`                      | `map[string]string` | `{}`    | Custom enter handlers for URI schemes |
| `todos.limiter.max_pending`          | `int`               | `0`     | Global pending-todo cap (`0` disables) |
| `todos.limiter.rate_limit_per_session` | `duration`        | `0`     | Per-session add cooldown (`0` disables) |
| `todos.notifications.toast`          | `bool`              | `true`  | Show toast on todo creation |

## Views

View-specific settings (keybindings, layout, behavior) are configured per-view under the `views` section.

### Sessions View

| Option                              | Type       | Default       | Description                                  |
| ----------------------------------- | ---------- | ------------- | -------------------------------------------- |
| `views.sessions.keybindings`        | `map`      |               | Key-to-command mappings                      |
| `views.sessions.split_ratio`        | `int`      | `25`          | List/preview split percentage (1-80)         |
| `views.sessions.refresh_interval`   | `duration` | `15s`         | Auto-refresh interval (0 to disable)         |
| `views.sessions.preview_enabled`    | `bool`     | `true`        | Enable tmux pane preview sidebar on startup  |
| `views.sessions.preview_title`      | `string`   |               | Go template for preview panel title          |
| `views.sessions.preview_status`     | `string`   |               | Go template for preview status line          |
| `views.sessions.group_by`           | `string`   | `repo`        | Tree view grouping: `repo` or `group`        |

### Tasks View

| Option                        | Type  | Default | Description                          |
| ----------------------------- | ----- | ------- | ------------------------------------ |
| `views.tasks.keybindings`     | `map` |         | Key-to-command mappings              |
| `views.tasks.split_ratio`     | `int` | `30`    | Tree/detail split percentage (1-80)  |

### Messages View

| Option                          | Type  | Default | Description                          |
| ------------------------------- | ----- | ------- | ------------------------------------ |
| `views.messages.keybindings`    | `map` |         | Key-to-command mappings              |
| `views.messages.split_ratio`    | `int` | `50`    | List/preview split percentage (1-80) |

### Global

| Option                        | Type  | Description                        |
| ----------------------------- | ----- | ---------------------------------- |
| `views.global.keybindings`    | `map` | Keybindings available in all views |

See [Keybindings](keybindings.md) for the full per-view configuration format and defaults.

## More Configuration

- **[Rules](rules.md)** — Repository-specific spawn, recycle, setup commands, and file copying
- **[User Commands](commands.md)** — Custom commands for the vim-style command palette
- **[Keybindings](keybindings.md)** — Per-view key mappings and palette commands
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
│   └── myproject-feature1-abc123/
└── context/                   # Per-repo context directories
    ├── {owner}/{repo}/        # Linked via .hive symlink
    └── shared/                # Shared context
```
