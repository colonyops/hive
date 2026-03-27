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

| Field            | Type             | Default            | Description                                       |
| ---------------- | ---------------- | ------------------ | ------------------------------------------------- |
| `pattern`        | string           | `""`               | Regex pattern to match remote URL                 |
| `clone_strategy` | string           | —                  | Override clone strategy for matching repos: `full` or `worktree` |
| `branch_template` | string          | `hive/{{ .Slug }}-{{ .ID }}` | Go template for the git branch name (worktree only). Variables: `.Name`, `.Slug`, `.Owner`, `.Repo`, `.ID`. The rendered value must be a valid git branch name (no spaces, colons, `~`, `^`, etc.) — session creation fails with a clear error if it isn't. |
| `windows`        | []WindowConfig   | see below          | Declarative tmux window layout (recommended)      |
| `spawn`          | []string         | —                  | Shell commands run after session creation (legacy) |
| `batch_spawn`    | []string         | —                  | Shell commands for batch session creation (legacy) |
| `recycle`        | []string         | git fetch/checkout/reset/clean | Commands run when recycling a session |
| `commands`       | []string         | `[]`               | Setup commands run after clone                    |
| `copy`           | []string         | `[]`               | Glob patterns for files to copy from parent repo  |
| `max_recycled`   | *int             | `5`                | Maximum recycled sessions to keep (0 = unlimited) |

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
| `rules[].commands`     | `.Path`, `.Name`, `.Slug`, `.ContextDir`, `.Owner`, `.Repo`, `.ID` |
| `rules[].recycle`      | `.DefaultBranch`                                                    |
| `rules[].branch_template` | `.Name`, `.Slug`, `.Owner`, `.Repo`, `.ID`                     |
| `usercommands.*.sh`    | `.Path`, `.Name`, `.Remote`, `.ID`, `.Tool`, `.TmuxWindow`, `.Args`, `.Form.*` |

!!! warning "Always use `shq` for shell quoting"
    Template variables like `.Name` and `.Path` may contain spaces or special characters. Always pipe them through `shq` (e.g., `{{ .Name | shq }}`) to prevent shell injection and word-splitting issues.

### `.Name` vs `.Slug` in `branch_template`

These two variables behave very differently as a branch name source:

| | `.Slug` | `.Name` |
|---|---|---|
| Value for session `"jalevin/fix auth"` | `jalevin-fix-auth` | `jalevin/fix auth` |
| Always a valid git branch name | ✅ | ❌ (spaces, colons, etc. are rejected by git) |
| Preserves `/` namespace separator | ❌ (converted to `-` for safe directory paths) | ✅ |

**Use `.Slug` when you want a safe, predictable branch name:**
```yaml
branch_template: "jalevin/{{ .Slug }}"   # jalevin/fix-auth-abc123
```

**Use `.Name` when your session names already follow git branch conventions** (alphanumeric, hyphens, slashes — no spaces or colons):
```yaml
branch_template: "{{ .Name }}"           # jalevin/fix-auth (if that's the session name)
```

If `.Name` renders to an invalid git branch name (e.g. contains a space), hive will fail with a clear error at session creation time rather than passing a bad name to git.

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
