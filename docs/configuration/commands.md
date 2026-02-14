# User Commands

## Command Palette

User commands provide a vim-style command palette accessible by pressing `:` in the TUI. This allows you to define custom commands that can be executed on selected sessions with arguments.

**Command Palette Features:**

- **Vim-style interface** — Press `:` to open the palette
- **Fuzzy filtering** — Type to filter commands (prefix and substring matching)
- **Arguments support** — Pass arguments to commands (e.g., `:review pr-123`)
- **Tab completion** — Auto-fill selected command name
- **Keyboard navigation** — `↑/k/ctrl+k`, `↓/j/ctrl+j`, `tab`, `enter`, `esc`

## Defining Commands

```yaml
usercommands:
  review:
    sh: "send-claude {{ .Name }} /review"
    help: "Send /review to Claude session"
    silent: true
  tidy:
    sh: "send-claude {{ .Name }} /tidy"
    help: "Send /tidy to Claude session"
    confirm: "Commit and push changes?"
  open:
    sh: "open {{ .Path }}"
    help: "Open session in Finder"
    silent: true
    exit: "true"
```

## Command Options

| Field     | Type           | Description                                                         |
| --------- | -------------- | ------------------------------------------------------------------- |
| `sh`      | string         | Shell command template (mutually exclusive with action)             |
| `action`  | string         | Built-in action: `recycle` or `delete` (mutually exclusive with sh) |
| `help`    | string         | Description shown in palette                                        |
| `confirm` | string         | Confirmation prompt (empty = no confirmation)                       |
| `silent`  | bool           | Skip loading popup for fast commands                                |
| `exit`    | string         | Exit TUI after command (bool or `$ENV_VAR`)                         |
| `form`    | `[]FormField`  | Interactive form fields collected before execution (see below)      |

## System Default Commands

Hive provides built-in commands that can be overridden in usercommands:

| Name        | Type   | Description                                            |
| ----------- | ------ | ------------------------------------------------------ |
| `Recycle`   | action | Recycles the selected session                          |
| `Delete`    | action | Deletes the selected session (or selected tmux window) |
| `SendBatch` | form   | Send a message to multiple agents via `agent-send`     |

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

Form commands don't require a focused session — they collect their own targets.

**Example:**

```yaml
usercommands:
  broadcast:
    sh: |
      {{ range .Form.targets }}
      {{ agentSend }} {{ .Name | shq }}:claude {{ $.Form.message | shq }}
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
      {{ agentSend }} {{ .Name | shq }}:claude {{ $.Form.message | shq }}
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

See the [Inter-Agent Code Review](../recipes/inter-agent-code-review.md) recipe for the full setup.

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
    sh: "{{ agentSend }} {{ .Name | shq }}:claude /tidy"
    help: "Send /tidy to agent"
    confirm: "Commit and push changes?"
  review:
    sh: "{{ agentSend }} {{ .Name | shq }}:claude /review"
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
