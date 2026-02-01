# Tmux Integration

Manage AI agent sessions in tmux with automated session creation, status monitoring, and convenient keybindings.

## Config

Add to your `~/.config/hive/config.yaml`:

```yaml
# Enable tmux status monitoring
integrations:
  terminal:
    enabled: [tmux]
    poll_interval: 500ms

# Use hive.sh script for session creation
commands:
  spawn:
    - ~/.config/tmux/layouts/hive.sh "{{ .Name }}" "{{ .Path }}"
  batch_spawn:
    - ~/.config/tmux/layouts/hive.sh -b "{{ .Name }}" "{{ .Path }}" "{{ .Prompt }}"

# Tmux-related keybindings
keybindings:
  enter:
    help: open/create tmux
    sh: ~/.config/tmux/layouts/hive.sh "{{ .Name }}" "{{ .Path }}"
    exit: $HIVE_POPUP
    silent: true
  p:
    help: popup
    sh: tmux display-popup -E -w 80% -h 80% "tmux new-session -s hive-popup -t '{{ .Name }}'"
    silent: true
  ctrl+d:
    help: kill session
    sh: tmux kill-session -t "{{ .Name }}" 2>/dev/null || true
  t:
    help: send /tidy
    sh: claude-send "{{ .Name }}:claude" "/tidy"
    silent: true
```

## Scripts

### hive.sh

Creates a tmux session with two windows: `claude` (running the AI) and `shell`. Supports background mode for batch session creation.

Save to `~/.config/tmux/layouts/hive.sh`:

```bash
#!/bin/bash
# Usage: hive.sh [-b] [session-name] [working-dir] [prompt]
#   -b: background mode (create session without attaching)

BACKGROUND=false
if [ "$1" = "-b" ]; then
    BACKGROUND=true
    shift
fi

SESSION="${1:-hive}"
WORKDIR="${2:-$PWD}"
PROMPT="${3:-}"

if [ -n "$PROMPT" ]; then
    CLAUDE_CMD="claude '$PROMPT'"
else
    CLAUDE_CMD="claude"
fi

if tmux has-session -t "$SESSION" 2>/dev/null; then
    [ "$BACKGROUND" = true ] && exit 0
    if [ -n "$TMUX" ]; then
        tmux switch-client -t "$SESSION"
    else
        tmux attach-session -t "$SESSION"
    fi
else
    tmux new-session -d -s "$SESSION" -n claude -c "$WORKDIR" "$CLAUDE_CMD"
    tmux new-window -t "$SESSION" -n shell -c "$WORKDIR"
    tmux select-window -t "$SESSION:claude"

    [ "$BACKGROUND" = true ] && exit 0
    if [ -n "$TMUX" ]; then
        tmux switch-client -t "$SESSION"
    else
        tmux attach-session -t "$SESSION"
    fi
fi
```

Make it executable:
```bash
chmod +x ~/.config/tmux/layouts/hive.sh
```

### claude-send

Sends text to a Claude session in tmux, useful for remote commands like `/tidy`.

Save to your `$PATH` (e.g., `~/bin/claude-send`):

```bash
#!/bin/bash
# Usage: claude-send <target> <text>
TARGET="${1:?Usage: claude-send <target> <text>}"
TEXT="${2:?Usage: claude-send <target> <text>}"

tmux send-keys -t "$TARGET" "$TEXT"
sleep 0.5
tmux send-keys -t "$TARGET" C-m
```

Make it executable:
```bash
chmod +x ~/bin/claude-send
```

## Tmux Config

Add to your `~/.tmux.conf`:

```bash
# Quick access to hive TUI as popup (prefix + Space)
bind Space display-popup -E -w 85% -h 85% "HIVE_POPUP=true hive"

# Quick switch to hive session
bind l switch-client -t hive
```

## Shell Alias

Add to your `.bashrc` or `.zshrc`:

```bash
# Start or attach to a persistent hive session
alias hv="tmux new-session -As hive hive"
```

## Usage

### Starting Hive

From anywhere:
```bash
hv  # Opens hive in a persistent tmux session
```

### In the TUI

- **Enter** - Opens or switches to the selected session's tmux session
- **p** - Opens the session in a tmux popup
- **Ctrl+d** - Kills the tmux session
- **t** - Sends `/tidy` command to the session

### Popup Mode

Press `prefix + Space` (in tmux) to open hive as a popup overlay. When you select a session with Enter from the popup, it will:
- Exit the popup (because `$HIVE_POPUP` is true)
- Switch to the selected session

### Remote Commands

Send commands to any session from anywhere:
```bash
claude-send "session-name:claude" "/tidy"
claude-send "session-name:claude" "explain this code"
```

## How It Works

1. **Status Monitoring**: Hive polls tmux windows every 500ms to detect agent status (working, waiting, needs approval)
2. **Session Management**: The hive.sh script creates/attaches tmux sessions with consistent layouts
3. **Keybindings**: Custom keybindings provide quick access without leaving the TUI
4. **Popup Integration**: Run hive as an overlay without dedicating a full window

## Status Indicators

When terminal integration is enabled, the TUI shows real-time agent status:

| Indicator | Color            | Meaning                         |
| --------- | ---------------- | ------------------------------- |
| `[●]`     | Green (animated) | Agent actively working          |
| `[!]`     | Yellow           | Agent needs approval/permission |
| `[>]`     | Cyan             | Agent ready for input           |
| `[?]`     | Dim              | Terminal session not found      |
| `[○]`     | Gray             | Session recycled                |
