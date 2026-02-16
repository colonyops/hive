# Workflow Configuration Examples

Complete hive configuration examples for common setups.

## Complete Config Example

```yaml
version: 0.2.5

repo_dirs:
  - ~/code/repos
  - ~/projects

tmux:
  poll_interval: 1.5s

tui:
  refresh_interval: 10s
  preview_enabled: true

rules:
  - pattern: ""
    max_recycled: 5
    windows:
      - name: "{{ agentWindow }}"
        command: '{{ agentCommand }} {{ agentFlags }}'
        focus: true
      - name: shell
    commands:
      - hive ctx init

  - pattern: ".*github\\.com/my-org/.*"
    windows:
      - name: claude
        command: "claude --model opus"
        focus: true
      - name: tests
        command: "npm run test:watch"
      - name: shell
    commands:
      - npm install
    copy:
      - .envrc

usercommands:
  review:
    sh: "{{ agentSend }} {{ .Name | shq }}:{{ agentWindow }} /review"
    help: "Send /review to Claude"
    silent: true

keybindings:
  r:
    cmd: Recycle
    confirm: "Recycle session?"
  d:
    cmd: Delete
```

## Organization-Specific Setup

```yaml
rules:
  # Work repos with setup script
  - pattern: ".*github\\.com/my-company/.*"
    windows:
      - name: claude
        command: "claude"
        focus: true
      - name: shell
    commands:
      - hive ctx init
      - make dev-setup
      - npm install
    copy:
      - .envrc

  # Personal repos (simple)
  - pattern: ".*github\\.com/my-username/.*"
    windows:
      - name: claude
        command: "claude"
        focus: true
      - name: shell
    commands:
      - hive ctx init
```

## Git Worktree Pattern

```yaml
rules:
  - pattern: ""
    windows:
      - name: claude
        command: "claude"
        focus: true
      - name: shell
    commands:
      - hive ctx init
      - bd init --stealth || true

usercommands:
  worktree:
    sh: 'cd {{ .Path }} && git worktree add ../{{ .Repo }}-{{ index .Args 0 }} {{ index .Args 0 }}'
    help: "Create worktree from branch"
```

## Custom Recycle Workflow

```yaml
rules:
  - pattern: ".*monorepo.*"
    recycle:
      - git fetch origin
      - git checkout -f main
      - git reset --hard origin/main
      - git clean -fd
      - npm install  # Reinstall after clean
```

## Rule Organization Best Practices

1. **Specific before general:** More specific patterns come first
2. **Test patterns:** Use `echo "url" | grep -E "pattern"` to test regex
3. **Document patterns:** Add comments explaining complex regex

```yaml
rules:
  # Critical production repos
  - pattern: ".*github\\.com/company/prod-.*"
    max_recycled: 1

  # Regular work repos
  - pattern: ".*github\\.com/company/.*"
    max_recycled: 3

  # Catch-all
  - pattern: ""
    max_recycled: 5
```

## User Command Patterns

```yaml
usercommands:
  # Send slash command to running agent
  tidy:
    sh: "send-claude {{ .Name }} /tidy"
    help: "Run /tidy in Claude session"
    confirm: "Commit and push changes?"
    silent: true

  # Open in editor
  vscode:
    sh: "code {{ .Path }}"
    help: "Open session in VS Code"
    silent: true
    exit: "true"

  # Git checkout
  checkout:
    sh: "cd {{ .Path }} && git checkout {{ index .Args 0 }}"
    help: "Checkout branch (usage: :checkout branch-name)"

  # Attach with exit condition
  attach:
    sh: "tmux attach -t {{ .Name }}"
    exit: "$HIVE_POPUP"  # Exit if HIVE_POPUP env var is true
```

## Exit Conditions

Control when TUI exits after command:

- `"true"` - Always exit
- `"false"` - Never exit
- `"$ENV_VAR"` - Exit if environment variable is set to "true"

## Command Safety Tips

1. **Quote variables:** Use `{{ .Variable | shq }}` for shell safety
2. **Handle failures:** Use `|| true` for optional commands
3. **Test commands:** Run manually before adding to config

```yaml
commands:
  - npm install || echo "npm not found"
  - test -f .envrc && direnv allow
```

## Migration Notes

### Key Changes in 0.2.5

- **YAML window config:** New `windows` field on rules replaces shell-based `spawn` commands for tmux. Declarative window definitions with `name`, `command`, `dir`, and `focus` fields
- **`TmuxOpen` / `TmuxStart` actions:** New built-in actions for opening (foreground) or starting (background) tmux sessions using window config
- **Default spawn uses windows:** When no rule specifies `windows` or `spawn`, the default is now a declarative window layout (agent window + shell)
- **Multi-window tree items:** Sessions with multiple agent windows now show each window as a selectable sub-item
- **TmuxWindow template variable:** `{{ .TmuxWindow }}` available in user commands for window targeting

### Key Changes in 0.2.4

- **Keybindings:** Must reference commands via `cmd` field
- **Built-in commands:** `Recycle` and `Delete` are now user commands
- **Actions removed:** Use `cmd: Recycle` instead of `action: recycle`
