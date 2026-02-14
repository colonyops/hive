# Plugins

Hive supports plugins that extend functionality with custom commands and status providers.

## Claude Plugin

The Claude plugin provides integration with Claude Code sessions:

- **ClaudeFork** — Fork the current Claude session in a new tmux window with conversation history
- **Analytics Status** — Display context usage with color warnings (yellow at 60%, red at 80%)

### Configuration

```yaml
plugins:
  claude:
    enabled: true # auto-detected, true/false to override
    cache_ttl: 30s # status cache duration
    yellow_threshold: 60 # yellow above this % (default: 60)
    red_threshold: 80 # red above this % (default: 80)
    model_limit: 200000 # context limit (default: 200000 for Sonnet)
```

### Usage

```yaml
# Add keybinding for fork
keybindings:
  f:
    cmd: ClaudeFork

# Or invoke via command palette
:ClaudeFork
```

### Context Analytics

The plugin displays session names in color based on context usage:

- **Default color**: < 60% (no warning)
- **Yellow**: 60-79% (approaching limit)
- **Red**: ≥ 80% (at/near limit)

### Requirements

- Claude CLI installed (`claude`)
- Claude session metadata stored in session (see spawn configuration below)

### Spawn Configuration

```yaml
rules:
  - pattern: ""
    spawn:
      - 'tmux new-window -c "{{ .Path }}" "exec claude"'
```

The Claude plugin automatically detects active session IDs by scanning `~/.claude/projects/{project-dir}/` for the most recently modified UUID session file (within 5 minutes). No manual metadata configuration needed.

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
