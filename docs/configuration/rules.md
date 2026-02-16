---
icon: lucide/regex
---

# Rules

Rules match repositories by regex pattern against the remote URL and configure how sessions are created, recycled, and set up. Rules are evaluated in order — the last matching rule with spawn/windows config wins. An empty pattern (`""`) matches all repositories.

```yaml
rules:
  # Default rule — windows config creates tmux sessions declaratively
  - pattern: ""
    max_recycled: 5
    windows:
      - name: "{{ agentWindow }}"
        command: '{{ agentCommand }} {{ agentFlags }}'
        focus: true
      - name: shell
    commands:
      - hive ctx init

  # Override for work repos — custom agent flags
  - pattern: ".*/my-org/.*"
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
```

!!! info "Last match wins"
    Rules are evaluated in order — the **last** matching rule with spawn/windows config wins. Place general patterns before specific ones.

## Rule Fields

| Field           | Type             | Default            | Description                                       |
| --------------- | ---------------- | ------------------ | ------------------------------------------------- |
| `pattern`       | string           | `""`               | Regex pattern to match remote URL                 |
| `windows`       | []WindowConfig   | see below          | Declarative tmux window layout (recommended)      |
| `spawn`         | []string         | —                  | Shell commands run after session creation (legacy) |
| `batch_spawn`   | []string         | —                  | Shell commands for batch session creation (legacy) |
| `recycle`       | []string         | git fetch/checkout/reset/clean | Commands run when recycling a session |
| `commands`      | []string         | `[]`               | Setup commands run after clone                    |
| `copy`          | []string         | `[]`               | Glob patterns for files to copy from parent repo  |
| `max_recycled`  | *int             | `5`                | Maximum recycled sessions to keep (0 = unlimited) |

!!! warning "`windows` vs `spawn`/`batch_spawn`"
    A rule must use either `windows` or `spawn`/`batch_spawn`, not both. If neither is set, the default window layout is used (agent window + shell window).

## Window Configuration

The `windows` field defines tmux windows declaratively. Each window has:

| Field     | Type   | Required | Default       | Description                                    |
| --------- | ------ | -------- | ------------- | ---------------------------------------------- |
| `name`    | string | yes      | —             | Window name (supports templates)               |
| `command` | string | no       | default shell | Command to run in the window (supports templates) |
| `dir`     | string | no       | session path  | Working directory override (supports templates) |
| `focus`   | bool   | no       | `false`       | Select this window after creation              |

**Default window layout** (used when no rule specifies `windows` or `spawn`):

```yaml
windows:
  - name: "{{ agentWindow }}"
    command: '{{ agentCommand }} {{ agentFlags }}'
    focus: true
  - name: shell
```

### Window Examples

**Agent + shell (default equivalent):**

```yaml
windows:
  - name: claude
    command: "claude"
    focus: true
  - name: shell
```

**Agent + test watcher + shell:**

```yaml
windows:
  - name: claude
    command: "claude --model opus"
    focus: true
  - name: tests
    command: "npm run test:watch"
  - name: shell
```

**Multiple agents:**

```yaml
windows:
  - name: claude
    command: "claude"
    focus: true
  - name: aider
    command: "aider --model sonnet"
  - name: shell
```

## Template Variables

Commands support Go templates with `{{ .Variable }}` syntax and `{{ .Variable | shq }}` for shell-safe quoting.

| Context                | Variables                                                           |
| ---------------------- | ------------------------------------------------------------------- |
| `rules[].windows`      | `.Path`, `.Name`, `.Slug`, `.ContextDir`, `.Owner`, `.Repo`, `.Prompt` (batch) |
| `rules[].spawn`        | `.Path`, `.Name`, `.Slug`, `.ContextDir`, `.Owner`, `.Repo`         |
| `rules[].batch_spawn`  | Same as spawn, plus `.Prompt`                                       |
| `rules[].recycle`      | `.DefaultBranch`                                                    |
| `usercommands.*.sh`    | `.Path`, `.Name`, `.Remote`, `.ID`, `.Tool`, `.TmuxWindow`, `.Args`, `.Form.*` |

!!! warning "Always use `shq` for shell quoting"
    Template variables like `.Name` and `.Path` may contain spaces or special characters. Always pipe them through `shq` (e.g., `{{ .Name | shq }}`) to prevent shell injection and word-splitting issues.

## Template Functions

| Function    | Description                                   |
| ----------- | --------------------------------------------- |
| `shq`       | Shell-quote a string for safe use in commands |
| `join`      | Join string slice with separator              |
| `hiveTmux`  | Path to bundled `hive-tmux` script            |
| `agentSend` | Path to bundled `agent-send` script           |
| `agentCommand` | Default agent command from `agents.default` profile |
| `agentWindow`  | Default agent window/profile name (use for targets like `session:{{ agentWindow }}`) |
| `agentFlags`   | Default agent flags from `agents.default` profile |
