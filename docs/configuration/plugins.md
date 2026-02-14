---
icon: lucide/puzzle
---

# Plugins

Plugins extend hive with custom commands and status providers. Plugins auto-detect their dependencies at startup — if the required CLI tool is installed, the plugin activates automatically.

```yaml
plugins:
  tmux:
    enabled: true
  claude:
    enabled: true
    yellow_threshold: 60
    red_threshold: 80
  github:
    enabled: true
    results_cache: 8m
  beads:
    enabled: true
    results_cache: 30s
```

!!! info "Auto-detection"
    Most plugins auto-detect their dependencies at startup. You only need to set `enabled: true` — if the required CLI tool isn't installed, the plugin silently deactivates. No errors, no configuration needed.

## Tmux Plugin

The tmux plugin provides default commands for session management using bundled scripts (`hive-tmux`, `agent-send`) that are auto-extracted to `$HIVE_DATA_DIR/bin/`.

```yaml
plugins:
  tmux:
    enabled: true # true by default, set false to disable
```

### Commands Provided

| Command          | Description                     | Default Key |
| ---------------- | ------------------------------- | ----------- |
| `TmuxOpen`       | Open/attach tmux session        | `enter`     |
| `TmuxStart`      | Start tmux session (background) | —           |
| `TmuxKill`       | Kill tmux session               | `ctrl+d`    |
| `TmuxPopUp`      | Popup tmux session              | `p`         |
| `AgentSend`      | Send Enter to agent             | `A`         |
| `AgentSendClear` | Send /clear to agent            | —           |

## Claude Plugin

The Claude plugin provides integration with Claude Code sessions — forking sessions and monitoring context usage.

```yaml
plugins:
  claude:
    enabled: true        # auto-detected (requires `claude` CLI)
    cache_ttl: 30s       # status cache duration
    yellow_threshold: 60 # yellow warning above this % (default: 60)
    red_threshold: 80    # red warning above this % (default: 80)
    model_limit: 200000  # context token limit (default: 200000)
```

### Commands Provided

| Command      | Description                      | Default Key |
| ------------ | -------------------------------- | ----------- |
| `ClaudeFork` | Fork Claude session in new window | —          |

### Context Analytics

Session names are colored based on context usage:

- **Default color** — Below 60% (no warning)
- **Yellow** — 60-79% (approaching limit)
- **Red** — 80%+ (at or near limit)

!!! note
    The plugin detects active session IDs by scanning `~/.claude/projects/{project-dir}/` for recently modified UUID session files (within 5 minutes). No manual metadata configuration needed.

### Usage

```yaml
# Add keybinding for fork
keybindings:
  f:
    cmd: ClaudeFork

# Or invoke via command palette
# :ClaudeFork
```

## GitHub Plugin

The GitHub plugin provides PR status display and GitHub CLI commands. Auto-detected when `gh` CLI is installed.

```yaml
plugins:
  github:
    enabled: true      # auto-detected (requires `gh` CLI)
    results_cache: 8m  # how often to refresh PR status (default: 8m)
```

### Commands Provided

| Command          | Description                | Default Key |
| ---------------- | -------------------------- | ----------- |
| `GithubOpenRepo` | Open repo in browser       | —           |
| `GithubOpenPR`   | View current PR in browser | —           |
| `GithubPRStatus` | Show PR status (popup)     | —           |
| `GithubPRCreate` | Create PR in browser       | —           |

### Status Display

Sessions with an associated PR show a status indicator:

| Label    | Color   | Meaning          |
| -------- | ------- | ---------------- |
| `PR open`   | Green   | PR is open       |
| `PR draft`  | Muted   | PR is a draft    |
| `PR merged` | Primary | PR was merged    |
| `PR closed` | Muted   | PR was closed    |

## Beads Plugin

The Beads plugin provides issue tracking integration with the `bd` (beads) CLI. Auto-detected when `bd` CLI is installed.

```yaml
plugins:
  beads:
    enabled: true        # auto-detected (requires `bd` CLI)
    results_cache: 30s   # how often to refresh issue counts (default: 30s)
```

### Commands Provided

| Command      | Description              | Default Key |
| ------------ | ------------------------ | ----------- |
| `BeadsReady` | Show ready tasks (popup) | —           |
| `BeadsList`  | List all issues (popup)  | —           |
| `BeadsTUI`   | Open perles kanban TUI   | —           |

`BeadsTUI` is only registered if the `perles` CLI is available.

### Status Display

Sessions with a `.beads` directory show issue progress:

| Display  | Color   | Meaning                    |
| -------- | ------- | -------------------------- |
| `BD 3/5` | Primary | 3 of 5 issues closed       |
| `BD 5/5` | Green   | All issues closed          |

## LazyGit Plugin

The lazygit plugin provides commands to open lazygit in a tmux popup. Auto-detected when `lazygit` is installed.

```yaml
plugins:
  lazygit:
    enabled: true  # auto-detected (requires `lazygit`)
```

### Commands Provided

| Command          | Description                 | Default Key |
| ---------------- | --------------------------- | ----------- |
| `LazyGitOpen`    | Open lazygit (full popup)   | —           |
| `LazyGitCommits` | Open lazygit commit log     | —           |

## Neovim Plugin

The neovim plugin provides a command to open neovim in the session's tmux session. Auto-detected when `nvim` is installed.

```yaml
plugins:
  neovim:
    enabled: true  # auto-detected (requires `nvim`)
```

### Commands Provided

| Command      | Description                          | Default Key |
| ------------ | ------------------------------------ | ----------- |
| `NeovimOpen` | Open neovim in new tmux window       | —           |

## Context Directory Plugin

The context directory plugin provides commands to open context directories in the system file browser. Always available on macOS and Linux.

```yaml
plugins:
  contextdir:
    enabled: true  # always available on macOS/Linux
```

### Commands Provided

| Command              | Description                      | Default Key |
| -------------------- | -------------------------------- | ----------- |
| `ContextOpenSession` | Open session's `.hive` directory | —           |
| `ContextOpenAll`     | Open all context directories     | —           |
