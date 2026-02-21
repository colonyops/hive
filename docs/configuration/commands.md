---
icon: lucide/terminal
---

# User Commands

User commands provide a vim-style command palette accessible by pressing `:` in the TUI. Define custom commands that execute shell scripts, display interactive forms, and integrate with tmux to control agents.

```yaml
usercommands:
  review:
    sh: "{{ agentSend }} {{ .Name | shq }}:{{ agentWindow }} /review"
    help: "Send /review to Claude session"
    silent: true
  tidy:
    sh: "{{ agentSend }} {{ .Name | shq }}:{{ agentWindow }} /tidy"
    help: "Send /tidy to Claude session"
    confirm: "Commit and push changes?"
  open:
    sh: "open {{ .Path }}"
    help: "Open session in Finder"
    silent: true
    exit: "true"
```

## Command Palette Features

- **Vim-style interface** — Press `:` to open the palette
- **Fuzzy filtering** — Type to filter commands (prefix and substring matching)
- **Arguments support** — Pass arguments to commands (e.g., `:review pr-123`)
- **Tab completion** — Auto-fill selected command name
- **Keyboard navigation** — `↑/k/ctrl+k`, `↓/j/ctrl+j`, `tab`, `enter`, `esc`

## Command Options

| Field     | Type                  | Description                                                         |
| --------- | --------------------- | ------------------------------------------------------------------- |
| `sh`      | string                | Shell command template (mutually exclusive with `action`)           |
| `action`  | string                | Built-in action name (mutually exclusive with `sh` and `windows`; see [Built-in Actions](#built-in-actions)) |
| `windows` | `[]WindowConfig`      | Tmux windows to open after `sh` completes (see [Multi-agent Workflows](#multi-agent-workflows)) |
| `options` | `UserCommandOptions`  | Execution options for window-based commands (see below)             |
| `help`    | string                | Description shown in palette                                        |
| `confirm` | string                | Confirmation prompt (empty = no confirmation)                       |
| `silent`  | bool                  | Skip loading popup for fast commands                                |
| `exit`    | string                | Exit TUI after command (bool or `$ENV_VAR`)                         |
| `form`    | `[]FormField`         | Interactive form fields collected before execution (see below)      |

## Built-in Actions

Actions are built-in behaviors that can be assigned to user commands via the `action` field. Names are case-insensitive.

### Session Management

| Action          | Description                        |
| --------------- | ---------------------------------- |
| `Recycle`       | Recycle the selected session       |
| `Delete`        | Delete the selected session        |
| `NewSession`    | Create a new session               |
| `RenameSession` | Rename the selected session        |

### Tmux

These actions are provided by the tmux plugin and require tmux to be available.

| Action      | Description                             |
| ----------- | --------------------------------------- |
| `TmuxOpen`  | Open/attach the session's tmux session  |
| `TmuxStart` | Start a tmux session in the background  |

### Filtering

| Action           | Description                          |
| ---------------- | ------------------------------------ |
| `FilterAll`      | Show all sessions                    |
| `FilterActive`   | Show sessions with active agents     |
| `FilterApproval` | Show sessions needing approval       |
| `FilterReady`    | Show sessions with idle agents       |

### Grouping

| Action        | Description                                |
| ------------- | ------------------------------------------ |
| `GroupSet`    | Set/clear the selected session's group     |
| `GroupToggle` | Toggle between repo and group tree view    |

### Navigation

| Action       | Description                    |
| ------------ | ------------------------------ |
| `NextActive` | Jump to next active session    |
| `PrevActive` | Jump to previous active session |

### Other

| Action      | Description                  |
| ----------- | ---------------------------- |
| `DocReview` | Open the document review view |
| `SetTheme`  | Preview and select a theme   |
| `Notifications`  | Show notification history    |

## System Default Commands

Hive registers several built-in actions as default commands that can be overridden in `usercommands`. Additionally, the `SendBatch` command provides a form-based workflow for messaging multiple agents at once.

!!! warning
    `action` is mutually exclusive with `sh` and `windows`. A command must have at least one of `action`, `sh`, or `windows`.

## Multi-agent Workflows

The `windows` field opens one or more tmux windows in the current session after `sh` completes. Each window runs an independent agent or process, enabling parallel multi-agent workflows from a single command.

### Window Fields

| Field     | Type   | Description                                             |
| --------- | ------ | ------------------------------------------------------- |
| `name`    | string | Window name (required, template string)                 |
| `command` | string | Command to run in the window (template string)          |
| `dir`     | string | Working directory override (template string)            |
| `focus`   | bool   | Select this window after creation                       |

### Options Fields

The `options` block controls how window-based commands execute:

| Field          | Type   | Description                                                              |
| -------------- | ------ | ------------------------------------------------------------------------ |
| `session_name` | string | Template string — when set, creates a new hive session before opening windows |
| `remote`       | string | Remote URL override for new session creation (requires `session_name`)   |
| `background`   | bool   | Open windows without attaching to the tmux session                       |

```yaml
usercommands:
  CodeReview:
    help: "parallel review: two specialists coordinated by a leader"
    silent: true
    windows:
      - name: leader
        focus: true
        command: claude 'coordinate a review...'
      - name: specialist-a
        command: codex 'review for correctness...'
      - name: specialist-b
        command: cursor 'review for security...'
```

See the [Recipes](../recipes/index.md) for full multi-agent workflow examples.

## Using Arguments

Arguments passed in the command palette are available via the `.Args` template variable:

```yaml
usercommands:
  msg:
    sh: |
      hive msg pub -t agent.{{ .ID }}.inbox "{{ range .Args }}{{ . }} {{ end }}"
    help: "Send message to session inbox"
```

Usage: `:msg hello world` → sends "hello world" to the session inbox

## Exit Conditions

The `exit` field supports environment variables for conditional behavior:

```yaml
usercommands:
  attach:
    sh: "tmux attach -t {{ .Name }}"
    exit: "$HIVE_POPUP" # Only exit if HIVE_POPUP=true
```

This is useful when running hive in a tmux popup vs a dedicated session.

## Form Fields

Commands with `form` fields display an interactive dialog before execution. Form values are available under `.Form.<variable>` in the shell template.

| Field         | Type     | Description                                                     |
| ------------- | -------- | --------------------------------------------------------------- |
| `variable`    | string   | Template variable name (accessed as `.Form.<variable>`)         |
| `type`        | string   | `text`, `textarea`, `select`, or `multi-select` (one of type/preset) |
| `preset`      | string   | `SessionSelector` or `ProjectSelector` (one of type/preset)    |
| `label`       | string   | Display label for the field                                     |
| `placeholder` | string   | Placeholder text (text/textarea)                                |
| `default`     | string   | Default value (text/textarea/select)                            |
| `options`     | []string | Static options (select/multi-select)                            |
| `multi`       | bool     | Enable multi-select (presets only)                              |
| `filter`      | string   | `active` (default) or `all` — SessionSelector only             |

Preset fields populate from runtime data. `SessionSelector` shows active sessions with running tmux sessions (grouped by project when multiple remotes exist). `ProjectSelector` shows discovered repositories.

!!! info
    Form commands don't require a focused session — they collect their own targets via preset fields like `SessionSelector` and `ProjectSelector`.

```yaml
usercommands:
  broadcast:
    sh: |
      {{ range .Form.targets }}
      {{ agentSend }} {{ .Name | shq }}:{{ agentWindow }} {{ $.Form.message | shq }}
      {{ end }}
    form:
      - variable: targets
        preset: SessionSelector
        multi: true
        label: "Select recipients"
      - variable: message
        type: text
        label: "Message"
        placeholder: "Type your message..."
    help: "send message to multiple agents"
    silent: true
```

## What You Can Build

User commands combined with tmux integration and messaging create a powerful extensibility layer — like NeoVim's Lua config for AI agent management. Here are real-world patterns:

### Batch Send to Multiple Agents

Send the same instruction to all active agents at once:

```yaml
usercommands:
  broadcast:
    sh: |
      {{ range .Form.targets }}
      {{ agentSend }} {{ .Name | shq }}:{{ agentWindow }} {{ $.Form.message | shq }}
      {{ end }}
    form:
      - variable: targets
        preset: SessionSelector
        multi: true
        label: "Select agents"
      - variable: message
        type: text
        label: "Message"
        placeholder: "run tests and report results"
    help: "Send message to multiple agents"
    silent: true
```

### Code Review Agent

Spin up a dedicated reviewer that reviews your branch and sends feedback via inbox:

```yaml
usercommands:
  ReviewRequest:
    sh: "~/.config/hive/scripts/request-review.sh {{ .Name }}"
    help: "Request code review of current branch"
  CheckInbox:
    sh: "hive msg sub -t agent.{{ .ID }}.inbox --new"
    help: "Check inbox for messages"
```

See the [Recipes](../recipes/index.md) for full multi-agent workflow examples.

### Quick Actions

Common operations bound to single keys:

```yaml
usercommands:
  open:
    sh: "open {{ .Path }}"
    help: "Open in Finder"
    silent: true
    exit: "true"
  tidy:
    sh: "{{ agentSend }} {{ .Name | shq }}:{{ agentWindow }} /tidy"
    help: "Send /tidy to agent"
    confirm: "Commit and push changes?"
  review:
    sh: "{{ agentSend }} {{ .Name | shq }}:{{ agentWindow }} /review"
    help: "Send /review to agent"
    silent: true

keybindings:
  o:
    cmd: open
  t:
    cmd: tidy
  R:
    cmd: review
```

### Conditional Exit

Exit hive after opening a session — useful when running hive in a tmux popup:

```yaml
usercommands:
  attach:
    sh: "tmux attach -t {{ .Name }}"
    exit: "$HIVE_POPUP" # Only exit if HIVE_POPUP=true
```
