# Plugins

Hive supports plugins that extend functionality with custom commands and status providers. Plugins auto-detect their dependencies at startup — if the required CLI tool is installed, the plugin activates automatically.

## Tmux Plugin

The tmux plugin provides default commands for session management using bundled scripts (`hive-tmux`, `agent-send`) that are auto-extracted to `$HIVE_DATA_DIR/bin/`.

### Commands Provided

| Command          | Description                     | Default Key |
| ---------------- | ------------------------------- | ----------- |
| `TmuxOpen`       | Open/attach tmux session        | `enter`     |
| `TmuxStart`      | Start tmux session (background) | —           |
| `TmuxKill`       | Kill tmux session               | `ctrl+d`    |
| `TmuxPopUp`      | Popup tmux session              | `p`         |
| `AgentSend`      | Send Enter to agent             | `A`         |
| `AgentSendClear` | Send /clear to agent            | —           |

### Configuration

```yaml
plugins:
  tmux:
    enabled: true # true by default, set false to disable
```

## Claude Plugin

The Claude plugin provides integration with Claude Code sessions.

### Features

- **ClaudeFork** — Fork the current Claude session in a new tmux window with conversation history
- **Context Analytics** — Display context usage with color warnings in the session tree

### Commands Provided

| Command      | Description                      | Default Key |
| ------------ | -------------------------------- | ----------- |
| `ClaudeFork` | Fork Claude session in new window | —          |

### Context Analytics

Session names are colored based on context usage:

- **Default color** — Below 60% (no warning)
- **Yellow** — 60-79% (approaching limit)
- **Red** — 80%+ (at or near limit)

The plugin detects active session IDs by scanning `~/.claude/projects/{project-dir}/` for recently modified UUID session files (within 5 minutes). No manual metadata configuration needed.

### Configuration

```yaml
plugins:
  claude:
    enabled: true        # auto-detected (requires `claude` CLI)
    cache_ttl: 30s       # status cache duration
    yellow_threshold: 60 # yellow warning above this % (default: 60)
    red_threshold: 80    # red warning above this % (default: 80)
    model_limit: 200000  # context token limit (default: 200000)
```

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

### Features

- **PR Status** — Shows PR state (open, draft, merged, closed) next to session names in the tree view
- **GitHub Commands** — Quick access to common `gh` operations via command palette

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

### Configuration

```yaml
plugins:
  github:
    enabled: true      # auto-detected (requires `gh` CLI)
    results_cache: 8m  # how often to refresh PR status (default: 8m)
```

## Beads Plugin

The Beads plugin provides issue tracking integration with the `bd` (beads) CLI. Auto-detected when `bd` CLI is installed.

### Features

- **Issue Progress** — Shows closed/total issue count next to session names
- **Issue Commands** — Quick access to issue listing and ready work via command palette
- **Perles TUI** — If the `perles` CLI is installed, provides a kanban board popup

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

### Configuration

```yaml
plugins:
  beads:
    enabled: true        # auto-detected (requires `bd` CLI)
    results_cache: 30s   # how often to refresh issue counts (default: 30s)
```

## LazyGit Plugin

The lazygit plugin provides commands to open lazygit in a tmux popup. Auto-detected when `lazygit` is installed.

### Commands Provided

| Command          | Description                 | Default Key |
| ---------------- | --------------------------- | ----------- |
| `LazyGitOpen`    | Open lazygit (full popup)   | —           |
| `LazyGitCommits` | Open lazygit commit log     | —           |

### Configuration

```yaml
plugins:
  lazygit:
    enabled: true  # auto-detected (requires `lazygit`)
```

## Neovim Plugin

The neovim plugin provides a command to open neovim in the session's tmux session. Auto-detected when `nvim` is installed.

### Commands Provided

| Command      | Description                          | Default Key |
| ------------ | ------------------------------------ | ----------- |
| `NeovimOpen` | Open neovim in new tmux window       | —           |

### Configuration

```yaml
plugins:
  neovim:
    enabled: true  # auto-detected (requires `nvim`)
```

## Context Directory Plugin

The context directory plugin provides commands to open context directories in the system file browser. Always available on macOS and Linux.

### Commands Provided

| Command              | Description                      | Default Key |
| -------------------- | -------------------------------- | ----------- |
| `ContextOpenSession` | Open session's `.hive` directory | —           |
| `ContextOpenAll`     | Open all context directories     | —           |

### Configuration

```yaml
plugins:
  contextdir:
    enabled: true  # always available on macOS/Linux
```
