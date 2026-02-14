# Configuration

Config file: `~/.config/hive/config.yaml`

## Minimal Example

```yaml
repo_dirs:
  - ~/projects

tmux:
  poll_interval: 500ms

rules:
  - pattern: ""
    max_recycled: 5
    commands:
      - hive ctx init
```

## Sections

### [Rules](rules.md)

Repository-specific actions for spawn, recycle, setup commands, and file copying. Rules match repositories by regex pattern against the remote URL. See [Rules](rules.md) for template variables and functions.

### [User Commands](commands.md)

Custom commands accessible via the vim-style command palette (`:` key). Commands can execute shell scripts, display interactive forms, and integrate with tmux to control agents. See [User Commands](commands.md).

### [Keybindings](keybindings.md)

Map keys to user commands or built-in actions. See [Keybindings](keybindings.md) for defaults and configuration.

### [Plugins](plugins.md)

External service integrations — tmux session management, Claude analytics, GitHub status, and Beads issue tracking. See [Plugins](plugins.md).

### [Themes](themes.md)

Built-in color palettes and custom theme creation. See [Themes](themes.md).

## Configuration Options

| Option                            | Type                     | Default                                  | Description                                  |
| --------------------------------- | ------------------------ | ---------------------------------------- | -------------------------------------------- |
| `repo_dirs`                       | `[]string`               | `[]`                                     | Directories to scan for repositories         |
| `copy_command`                    | `string`                 | `pbcopy` (macOS)                         | Command to copy to clipboard                 |
| `rules`                           | `[]Rule`                 | `[]`                                     | Repository-specific setup rules              |
| `keybindings`                     | `map[string]Keybinding`  | See [keybindings](keybindings.md)        | TUI keybindings (reference usercommands)     |
| `usercommands`                    | `map[string]UserCommand` | Recycle, Delete, NewSession (system)     | Named commands for palette and keybindings   |
| `tui.theme`                       | `string`                 | `tokyo-night`                            | Built-in theme name                          |
| `tui.refresh_interval`            | `duration`               | `15s`                                    | Auto-refresh interval (0 to disable)         |
| `tui.preview_enabled`             | `bool`                   | `false`                                  | Enable tmux pane preview sidebar on startup  |
| `tmux.poll_interval`              | `duration`               | `500ms`                                  | Status check frequency (tmux always enabled) |
| `tmux.preview_window_matcher`     | `[]string`               | `["claude", "aider", "codex"]`           | Regex patterns for preferred window names    |
| `messaging.topic_prefix`          | `string`                 | `agent`                                  | Default prefix for topic IDs                 |
| `context.symlink_name`            | `string`                 | `.hive`                                  | Symlink name for context directories         |
| `plugins.tmux.enabled`            | `*bool`                  | `true`                                   | Enable/disable tmux plugin                   |
| `plugins.github.enabled`          | `*bool`                  | `nil` (auto-detect)                      | Enable/disable GitHub plugin                 |
| `plugins.github.results_cache`    | `duration`               | `8m`                                     | GitHub status polling interval               |
| `plugins.beads.enabled`           | `*bool`                  | `nil` (auto-detect)                      | Enable/disable Beads plugin                  |
| `plugins.beads.results_cache`     | `duration`               | `30s`                                    | Beads status polling interval                |
| `plugins.claude.enabled`          | `*bool`                  | `nil` (auto-detect)                      | Enable/disable Claude plugin                 |
| `plugins.claude.cache_ttl`        | `duration`               | `30s`                                    | Status cache duration                        |
| `plugins.claude.yellow_threshold` | `int`                    | `60`                                     | Yellow warning threshold (%)                 |
| `plugins.claude.red_threshold`    | `int`                    | `80`                                     | Red warning threshold (%)                    |
| `plugins.claude.model_limit`      | `int`                    | `200000`                                 | Context token limit                          |
| `plugins.lazygit.enabled`         | `*bool`                  | `nil` (auto-detect)                      | Enable/disable lazygit plugin                |
| `plugins.neovim.enabled`          | `*bool`                  | `nil` (auto-detect)                      | Enable/disable neovim plugin                 |
| `plugins.contextdir.enabled`      | `*bool`                  | `nil` (auto-detect)                      | Enable/disable context directory plugin      |

## Data Storage

All data is stored at `~/.local/share/hive/`:

```
~/.local/share/hive/
├── sessions.json              # Session state
├── bin/                       # Bundled scripts (auto-extracted)
│   ├── hive-tmux              # Tmux session launcher
│   └── agent-send             # Send text to agent in tmux
├── repos/                     # Cloned repositories
│   └── myproject-feature1-abc123/
├── context/                   # Per-repo context directories
│   ├── {owner}/{repo}/        # Linked via .hive symlink
│   └── shared/                # Shared context
└── messages/
    └── topics/                # Pub/sub message storage
```
