# Tmux Integration

Manage AI agent sessions in tmux with automated session creation, status monitoring, and convenient keybindings.

## Quick Start

Tmux integration works out of the box when `tmux` is on PATH. The tmux plugin activates automatically and provides:

- **Bundled scripts** — `hive-tmux` and `agent-send` are embedded in the binary and auto-extracted to `~/.local/share/hive/bin/`
- **Default spawn commands** — Sessions create tmux sessions automatically (no config needed)
- **Default keybindings** — `enter` to open, `ctrl+d` to kill, `p` for popup, `A` to send Enter

## Terminology Note

- **Hive session** - An isolated git clone + terminal environment managed by hive
- **Tmux session** - A terminal multiplexer session that hosts the hive session
- **Agent** - The AI tool (Claude, Aider, etc.) running within the tmux session

```
Hive Session "fix-bug" (ID: abc123)
  ↓ spawns
Tmux Session "fix-bug"
  ↓ contains windows
  ├─ Window: claude (runs the AI agent)
  └─ Window: shell (regular shell)
```

## Config

The defaults work for most setups. Override only what you need:

```yaml
# Tmux integration (always enabled)
tmux:
  poll_interval: 500ms
  # Regex patterns for preferred windows when capturing pane content
  preview_window_matcher: [claude, aider, codex]

# Tmux plugin (auto-detected)
plugins:
  tmux:
    enabled: true  # nil = auto-detect

# Override spawn commands if you don't want the bundled hive-tmux:
# rules:
#   - pattern: ""
#     spawn:
#       - 'wezterm cli spawn --cwd "{{ .Path }}" -- claude'
```

### Custom User Commands

You can add additional tmux-related commands:

```yaml
usercommands:
  send-review:
    sh: '{{ agentSend }} {{ .Name | shq }}:claude /review'
    help: "send /review to agent"
    silent: true
```

### Keybinding Overrides

Default keybindings can be overridden in your config:

```yaml
keybindings:
  enter:
    cmd: TmuxOpen       # default
  ctrl+d:
    cmd: TmuxKill       # default
  p:
    cmd: TmuxPopUp      # default
  A:
    cmd: AgentSend       # default
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
- **A** - Sends Enter to the agent
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

Send commands to any session using the bundled `agent-send` script:
```bash
~/.local/share/hive/bin/agent-send "session-name:claude" "/tidy"
~/.local/share/hive/bin/agent-send "session-name:claude" "explain this code"
```

Or add it to your PATH for convenience.

## Bundled Scripts

### hive-tmux

Creates a tmux session with two windows: `claude` (running the AI) and `shell`. Supports background mode for batch creation.

```
Usage: hive-tmux [-b] [session-name] [working-dir] [prompt]
  -b: background mode (create session without attaching)
```

### agent-send

Sends text to a tmux pane with a delayed Enter keystroke.

```
Usage: agent-send <target> [text]
  target: tmux target (session:window or session:window.pane)
  text: text to send (omit to just send Enter)

Environment:
  CLAUDE_SEND_DELAY: delay before Enter (default: 0.5s)
```

## How It Works

1. **Script Extraction**: On startup, hive extracts bundled scripts to `~/.local/share/hive/bin/` (re-extracts on version change)
2. **Status Monitoring**: Hive polls tmux windows every 500ms to detect agent status (working, waiting, needs approval)
3. **Multi-Window Discovery**: The `preview_window_matcher` patterns identify agent windows. When multiple windows match, each appears as a selectable tree item with its own status and preview
4. **Window Targeting**: The `{{ .TmuxWindow }}` template variable resolves to the selected window name, enabling keybindings to focus specific windows
5. **Keybindings**: The tmux plugin registers commands that are mapped to default keybindings, all user-configurable
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
