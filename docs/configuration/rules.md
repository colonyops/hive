---
icon: lucide/regex
---

# Rules

Rules match repositories by regex pattern against the remote URL and configure how sessions are created, recycled, and set up. Rules are evaluated in order — the last matching rule with spawn/windows config wins, and the last matching `agent` selects the agent profile. An empty pattern (`""`) matches all repositories.

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

  # Override for work repos — use the aider profile
  - pattern: ".*/my-org/.*"
    agent: aider
    windows:
      - name: "{{ agentWindow }}"
        command: "{{ agentCommand }} {{ agentFlags }}"
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
    Rules are evaluated in order — the **last** matching rule with spawn/windows config wins, and the **last** matching rule with `agent` selects the agent profile. Place general patterns before specific ones.

## Rule Fields

| Field              | Type           | Default                      | Description                                       |
| ------------------ | -------------- | ---------------------------- | ------------------------------------------------- |
| `pattern`          | string         | `""`                         | Regex pattern to match remote URL                 |
| `clone_strategy`   | string         | —                            | Override clone strategy for matching repos: `full` or `worktree` |
| `branch_template`  | string         | `hive/{{ .Slug }}-{{ .ID }}` | Go template for the git branch name (worktree only). Variables: `.Name`, `.Slug`, `.Owner`, `.Repo`, `.ID`. The rendered value must be a valid git branch name (no spaces, colons, `~`, `^`, etc.) — session creation fails with a clear error if it isn't. |
| `agent`            | string         | —                            | Agent profile override for matching repos. Must match a key under `agents`. |
| `windows`          | []WindowConfig | see below                    | Declarative tmux window layout (recommended)      |
| `spawn`            | []string       | —                            | Shell commands run after session creation (legacy) |
| `batch_spawn`      | []string       | —                            | Shell commands for batch session creation (legacy) |
| `recycle`          | []string       | git fetch/checkout/reset/clean | Commands run when recycling a session           |
| `commands`         | []string       | `[]`                         | Setup commands run after clone                    |
| `copy`             | []string       | `[]`                         | Glob patterns for files to copy from parent repo  |
| `max_recycled`     | *int           | `5`                          | Maximum recycled sessions to keep (0 = unlimited) |

!!! warning "`windows` vs `spawn`/`batch_spawn`"
    A rule must use either `windows` or `spawn`/`batch_spawn`, not both. If neither is set, the default window layout is used (agent window + shell window). A rule can set `agent` with or without custom `windows`/`spawn` config.

## Agent Overrides

Use `agent` to select a configured agent profile for repositories matching a rule. The value must match a profile under `agents`.

```yaml
agents:
  default: claude
  claude:
    command: claude
    flags: ["--model", "opus"]
  aider:
    command: aider
    flags: ["--model", "sonnet"]

rules:
  - pattern: ""
    max_recycled: 5

  # Uses the default window layout, rendered with the aider profile.
  - pattern: ".*/data-team/.*"
    agent: aider

  # Custom windows can still use the selected profile through template functions.
  - pattern: ".*/infra/.*"
    agent: aider
    windows:
      - name: "{{ agentWindow }}"
        command: "{{ agentCommand }} {{ agentFlags }}"
        focus: true
      - name: shell
```

Agent resolution order is:

1. CLI `--agent` flag
2. Last matching rule with `agent`
3. `agents.default`

## Window Configuration

The `windows` field defines tmux windows declaratively. Each window has:

| Field     | Type         | Required | Default       | Description                                    |
| --------- | ------------ | -------- | ------------- | ---------------------------------------------- |
| `name`    | string       | yes      | —             | Window name (supports templates)               |
| `command` | string       | no       | default shell | Command to run in the window (supports templates). Mutually exclusive with `panes`. |
| `dir`     | string       | no       | session path  | Working directory override (supports templates) |
| `focus`   | bool         | no       | `false`       | Select this window after creation              |
| `panes`   | []PaneConfig | no       | single pane   | Panes to create in the window. Mutually exclusive with `command`. |

!!! warning "`command` vs `panes`"
    A window must use either `command` or `panes`, not both. Use `command` for a single-pane window. Use `panes` when you need splits inside the window.

**Default window layout** (used when no rule specifies `windows` or `spawn`). The template functions render from the resolved agent profile:

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
  - name: "{{ agentWindow }}"
    command: "{{ agentCommand }} {{ agentFlags }}"
    focus: true
  - name: shell
```

**Agent + test watcher + shell:**

```yaml
windows:
  - name: "{{ agentWindow }}"
    command: "{{ agentCommand }} {{ agentFlags }}"
    focus: true
  - name: tests
    command: "npm run test:watch"
  - name: shell
```

**Multiple agents:**

```yaml
windows:
  - name: "{{ agentWindow }}"
    command: "{{ agentCommand }} {{ agentFlags }}"
    focus: true
  - name: aider
    command: "aider --model sonnet"
  - name: shell
```

**Split panes inside a window:**

```yaml
windows:
  - name: "{{ agentWindow }}"
    focus: true
    panes:
      - command: "{{ agentCommand }} {{ agentFlags }}"
      - command: "npm test -- --watch"
        split: horizontal
        size: 30%
  - name: shell
```

## Pane Configuration

The `panes` field defines tmux panes inside a window. The first pane becomes the initial pane for the window. Additional panes are created with `tmux split-window`.

| Field     | Type   | Required | Default       | Description                                    |
| --------- | ------ | -------- | ------------- | ---------------------------------------------- |
| `command` | string | no       | default shell | Command to run in the pane (supports templates) |
| `dir`     | string | no       | window/session path | Working directory override (supports templates) |
| `size`    | string | no       | tmux default  | Size passed to `tmux split-window -l` (for example `30%`) |
| `split`   | string | no       | `vertical`    | Split direction: `horizontal` or `vertical` |

## Template Variables

Commands support Go templates with `{{ .Variable }}` syntax and `{{ .Variable | shq }}` for shell-safe quoting.

| Context                | Variables                                                           |
| ---------------------- | ------------------------------------------------------------------- |
| `rules[].windows` / `panes` | `.Path`, `.Name`, `.Slug`, `.ContextDir`, `.Owner`, `.Repo`, `.Prompt` (batch) |
| `rules[].spawn`        | `.Path`, `.Name`, `.Slug`, `.ContextDir`, `.Owner`, `.Repo`         |
| `rules[].batch_spawn`  | Same as spawn, plus `.Prompt`                                       |
| `rules[].commands`     | `.Path`, `.Name`, `.Slug`, `.ContextDir`, `.Owner`, `.Repo`, `.ID` |
| `rules[].recycle`      | `.DefaultBranch`                                                    |
| `rules[].branch_template` | `.Name`, `.Slug`, `.Owner`, `.Repo`, `.ID`                     |
| `usercommands.*.sh`    | `.Path`, `.Name`, `.Remote`, `.ID`, `.Tool`, `.TmuxWindow`, `.Args`, `.Form.*`, `.Doc.Path`, `.Doc.RelPath`, `.Doc.Type` (review scope) |

!!! warning "Always use `shq` for shell quoting"
    Template variables like `.Name` and `.Path` may contain spaces or special characters. Always pipe them through `shq` (e.g., `{{ .Name | shq }}`) to prevent shell injection and word-splitting issues.

### `.Name` vs `.Slug` in `branch_template`

These two variables behave very differently as a branch name source:

| | `.Slug` | `.Name` |
| --- | --- | --- |
| Value for session `"dev/fix auth"` | `dev-fix-auth` | `dev/fix auth` |
| Always a valid git branch name | yes | no (spaces, colons, etc. are rejected by git) |
| Preserves `/` namespace separator | no (converted to `-` for safe directory paths) | yes |

**Use `.Slug` when you want a safe, predictable branch name:**
```yaml
branch_template: "dev/{{ .Slug }}"   # dev/fix-auth-abc123
```

**Use `.Name` when your session names already follow git branch conventions** (alphanumeric, hyphens, slashes — no spaces or colons):
```yaml
branch_template: "{{ .Name }}"           # dev/fix-auth (if that's the session name)
```

If `.Name` renders to an invalid git branch name (e.g. contains a space), hive will fail with a clear error at session creation time rather than passing a bad name to git.

## Template Functions

| Function    | Description                                   |
| ----------- | --------------------------------------------- |
| `shq`       | Shell-quote a string for safe use in commands |
| `join`      | Join string slice with separator              |
| `hiveTmux`  | Path to bundled `hive-tmux` script            |
| `agentSend` | Path to bundled `agent-send` script           |
| `agentCommand` | Command from the resolved agent profile |
| `agentWindow`  | Resolved agent profile name/window name (use for targets like `session:{{ agentWindow }}`) |
| `agentFlags`   | Flags from the resolved agent profile |
