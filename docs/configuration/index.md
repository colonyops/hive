# Configuration

Config file: `~/.config/hive/config.yaml`

```yaml
# Directories to scan for repositories (enables 'n' key in TUI)
repo_dirs:
  - ~/code/repos

# Terminal integration for real-time agent status (always enabled)
tmux:
  poll_interval: 500ms

# Rules for repository-specific setup
rules:
  # Default rule (empty pattern matches all)
  - pattern: ""
    max_recycled: 5
    # spawn/batch_spawn default to bundled hive-tmux script if not set:
    #   spawn: ['{{ hiveTmux }} {{ .Name | shq }} {{ .Path | shq }}']
    #   batch_spawn: ['{{ hiveTmux }} -b {{ .Name | shq }} {{ .Path | shq }} {{ .Prompt | shq }}']
    commands:
      - hive ctx init

  # Override spawn for work repos
  - pattern: ".*/my-org/.*"
    spawn:
      - '{{ hiveTmux }} {{ .Name | shq }} {{ .Path | shq }}'
    commands:
      - npm install
    copy:
      - .envrc
```

## Rules

Rules match repositories by regex pattern against the remote URL. The first matching rule wins. An empty pattern (`""`) matches all repositories.

Each rule can configure:

- **spawn** — Commands run after session creation (defaults to bundled `hive-tmux`)
- **batch_spawn** — Commands run after batch session creation (defaults to bundled `hive-tmux -b`)
- **recycle** — Commands run when recycling a session (defaults to git fetch/checkout/reset/clean)
- **commands** — Setup commands run after clone
- **copy** — Glob patterns for files to copy from the parent repo
- **max_recycled** — Maximum recycled sessions to keep (0 = unlimited)

## Template Variables

Commands support Go templates with `{{ .Variable }}` syntax and `{{ .Variable | shq }}` for shell-safe quoting.

| Context               | Variables                                                           |
| --------------------- | ------------------------------------------------------------------- |
| `rules[].spawn`       | `.Path`, `.Name`, `.Slug`, `.ContextDir`, `.Owner`, `.Repo`         |
| `rules[].batch_spawn` | Same as spawn, plus `.Prompt`                                       |
| `rules[].recycle`     | `.DefaultBranch`                                                    |
| `usercommands.*.sh`   | `.Path`, `.Name`, `.Remote`, `.ID`, `.Tool`, `.TmuxWindow`, `.Args`, `.Form.*` |

### Template Functions

| Function    | Description                                   |
| ----------- | --------------------------------------------- |
| `shq`       | Shell-quote a string for safe use in commands |
| `join`      | Join string slice with separator              |
| `hiveTmux`  | Path to bundled `hive-tmux` script            |
| `agentSend` | Path to bundled `agent-send` script           |

## Configuration Options

| Option                            | Type                     | Default                                  | Description                                  |
| --------------------------------- | ------------------------ | ---------------------------------------- | -------------------------------------------- |
| `repo_dirs`                       | `[]string`               | `[]`                                     | Directories to scan for repositories         |
| `copy_command`                    | `string`                 | `pbcopy` (macOS)                         | Command to copy to clipboard                 |
| `rules`                           | `[]Rule`                 | `[]`                                     | Repository-specific setup rules              |
| `rules[].pattern`                 | `string`                 | `""`                                     | Regex pattern to match remote URL            |
| `rules[].spawn`                   | `[]string`               | bundled `hive-tmux`                      | Commands after session creation              |
| `rules[].batch_spawn`             | `[]string`               | bundled `hive-tmux -b`                   | Commands after batch session creation        |
| `rules[].recycle`                 | `[]string`               | git fetch/checkout/reset/clean           | Commands when recycling a session            |
| `rules[].commands`                | `[]string`               | `[]`                                     | Setup commands to run after clone            |
| `rules[].copy`                    | `[]string`               | `[]`                                     | Glob patterns for files to copy              |
| `rules[].max_recycled`            | `*int`                   | `5`                                      | Max recycled sessions (0 = unlimited)        |
| `keybindings`                     | `map[string]Keybinding`  | See [commands](commands.md)              | TUI keybindings (reference usercommands)     |
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
