# Tmux Integration

Manage AI agent sessions in tmux with automated session creation, status monitoring, and convenient keybindings.

## Terminology Note

When using tmux integration, it's important to understand the distinction between:

- **Hive session** - An isolated git clone + terminal environment managed by hive
- **Tmux session** - A terminal multiplexer session that hosts the hive session
- **Agent** - The AI tool (Claude, Aider, etc.) running within the tmux session

When you create a hive session with tmux integration enabled, hive spawns a tmux session with the same name. The relationship is:

```
Hive Session "fix-bug" (ID: abc123)
  ↓ spawns
Tmux Session "fix-bug"
  ↓ contains windows
  ├─ Window: claude (runs the AI agent)
  └─ Window: shell (regular shell)
```

Throughout this guide, "session" refers to the hive session unless explicitly noted as "tmux session".

## Config

Add these tmux-specific settings to your `~/.config/hive/config.yaml` (not a complete config, just the tmux-related parts):

```yaml
# Tmux integration (always enabled)
tmux:
  poll_interval: 500ms
  # Regex patterns for preferred windows when capturing pane content
  # Hive will prioritize windows matching these patterns over the active window
  preview_window_matcher: [claude, aider, codex]

# Use hive.sh script for session creation
commands:
  spawn:
    - ~/.config/tmux/layouts/hive.sh {{ .Name | shq }} {{ .Path | shq }}
  batch_spawn:
    - ~/.config/tmux/layouts/hive.sh -b {{ .Name | shq }} {{ .Path | shq }} {{ .Prompt | shq }}

# User commands for tmux operations
usercommands:
  tmux-open:
    sh: ~/.config/tmux/layouts/hive.sh -w {{ .TmuxWindow | shq }} {{ .Name | shq }} {{ .Path | shq }}
    help: "open/create tmux"
    exit: $HIVE_POPUP
    silent: true
  tmux-popup:
    sh: tmux display-popup -E -w 80% -h 80% "tmux new-session -s hive-popup -t '{{ .Name }}'"
    help: "popup"
    silent: true
  tmux-kill:
    sh: tmux kill-session -t "{{ .Name }}" 2>/dev/null || true
    help: "kill session"
  send-tidy:
    sh: claude-send "{{ .Name }}:claude" "/tidy"
    help: "send /tidy"
    silent: true

# Keybindings reference the commands above
keybindings:
  enter:
    cmd: tmux-open
  p:
    cmd: tmux-popup
  ctrl+d:
    cmd: tmux-kill
  t:
    cmd: send-tidy
```

## Scripts

### hive.sh

Creates a tmux session with two windows: `claude` (running the AI) and `shell`. Supports background mode for batch creation and `-w` for targeting a specific window.

Save to `~/.config/tmux/layouts/hive.sh`:

```bash
#!/bin/bash
# Usage: hive.sh [-b] [-w window] [session-name] [working-dir] [prompt]
#   -b: background mode (create session without attaching)
#   -w: target window in an existing session

BACKGROUND=false
TARGET_WINDOW=""

while [ $# -gt 0 ]; do
    case "$1" in
        -b) BACKGROUND=true; shift ;;
        -w) TARGET_WINDOW="$2"; shift 2 ;;
        *)  break ;;
    esac
done

SESSION="${1:-hive}"
WORKDIR="${2:-$PWD}"
PROMPT="${3:-}"

if [ -n "$PROMPT" ]; then
    CLAUDE_CMD="claude '$PROMPT'"
else
    CLAUDE_CMD="claude"
fi

# Target a specific window in an existing session
if [ -n "$TARGET_WINDOW" ] && tmux has-session -t "$SESSION" 2>/dev/null; then
    [ "$BACKGROUND" = true ] && exit 0
    if [ -n "$TMUX" ]; then
        tmux switch-client -t "$SESSION:$TARGET_WINDOW"
    else
        tmux attach-session -t "$SESSION" \; select-window -t "$TARGET_WINDOW"
    fi
    exit 0
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

- **Enter** - Opens or switches to the selected session (or specific window)
- **p** - Opens the session in a tmux popup
- **Ctrl+d** - Kills the tmux session
- **d** - Deletes the selected session (or selected window)
- **t** - Sends `/tidy` command to the session
- **v** - Toggle preview sidebar
- **:** - Open command palette (filter by status, etc.)

### Multi-Window Sessions

When a tmux session has multiple agent windows (matched by `preview_window_matcher` patterns), each window appears as a selectable sub-item:

```
repo-name
├─ [●] my-session #abcd
│     ├─ claude (window 0)
│     └─ aider (window 1)
├─ [?] other-session #efgh
└─ Recycled (2)
```

Each window has its own status indicator and preview content. Pressing **Enter** on a window sub-item focuses that specific window via the `{{ .TmuxWindow }}` template variable.

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
2. **Multi-Window Discovery**: The `preview_window_matcher` patterns identify agent windows. When multiple windows match, each appears as a selectable tree item with its own status and preview
3. **Window Targeting**: The `{{ .TmuxWindow }}` template variable resolves to the selected window name, enabling keybindings to focus specific windows
4. **Session Management**: The hive.sh script creates/attaches tmux sessions with consistent layouts
5. **Keybindings**: Custom keybindings provide quick access without leaving the TUI
6. **Popup Integration**: Run hive as an overlay without dedicating a full window

## Filtering by Status

Use the command palette (`:`) to filter sessions by terminal status:

- `:FilterActive` - Show only sessions where the agent is actively working
- `:FilterApproval` - Show sessions waiting for user approval
- `:FilterReady` - Show sessions where the agent is idle
- `:FilterAll` - Clear filters and show all sessions

The active filter is displayed in the tab bar (e.g., `[active]`).

## Status Indicators

When terminal integration is enabled, the TUI shows real-time agent status:

| Indicator | Color            | Meaning                         |
| --------- | ---------------- | ------------------------------- |
| `[●]`     | Green (animated) | Agent actively working          |
| `[!]`     | Yellow           | Agent needs approval/permission |
| `[>]`     | Cyan             | Agent ready for input           |
| `[?]`     | Dim              | Terminal session not found      |
| `[○]`     | Gray             | Session recycled                |
