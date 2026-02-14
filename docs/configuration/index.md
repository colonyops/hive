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
    commands:
      - hive ctx init
```

## General Settings

| Option                        | Type       | Default              | Description                                 |
| ----------------------------- | ---------- | -------------------- | ------------------------------------------- |
| `repo_dirs`                   | `[]string` | `[]`                 | Directories to scan for repositories        |
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

| Option                 | Type       | Default        | Description                                  |
| ---------------------- | ---------- | -------------- | -------------------------------------------- |
| `tui.theme`            | `string`   | `tokyo-night`  | Built-in theme name (see [Themes](themes.md))|
| `tui.refresh_interval` | `duration` | `15s`          | Auto-refresh interval (0 to disable)         |
| `tui.preview_enabled`  | `bool`     | `true`         | Enable tmux pane preview sidebar on startup  |

## Messaging

| Option                   | Type     | Default  | Description                  |
| ------------------------ | -------- | -------- | ---------------------------- |
| `messaging.topic_prefix` | `string` | `agent`  | Default prefix for topic IDs |
| `context.symlink_name`   | `string` | `.hive`  | Symlink name for context dir |

## More Configuration

- **[Rules](rules.md)** — Repository-specific spawn, recycle, setup commands, and file copying
- **[User Commands](commands.md)** — Custom commands for the vim-style command palette
- **[Keybindings](keybindings.md)** — Map keys to user commands or built-in actions
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
