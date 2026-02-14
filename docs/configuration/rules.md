---
icon: lucide/regex
---

# Rules

Rules match repositories by regex pattern against the remote URL and configure how sessions are created, recycled, and set up. The first matching rule wins. An empty pattern (`""`) matches all repositories.

```yaml
rules:
  # Default rule (empty pattern matches all)
  - pattern: ""
    max_recycled: 5
    # spawn/batch_spawn default to bundled hive-tmux script if not set:
    #   spawn: ['{{ hiveTmux }} {{ .Name | shq }} {{ .Path | shq }}']
    #   batch_spawn: ['{{ hiveTmux }} -b {{ .Name | shq }} {{ .Path | shq }} {{ .Prompt | shq }}']
    commands:
      - hive ctx init

  # Override spawn for work repos
  - pattern: ".*/my-org/.*"
    spawn:
      - '{{ hiveTmux }} {{ .Name | shq }} {{ .Path | shq }}'
    commands:
      - npm install
    copy:
      - .envrc
```

## Rule Fields

| Field           | Type       | Default                        | Description                                       |
| --------------- | ---------- | ------------------------------ | ------------------------------------------------- |
| `pattern`       | string     | `""`                           | Regex pattern to match remote URL                 |
| `spawn`         | []string   | bundled `hive-tmux`            | Commands run after session creation               |
| `batch_spawn`   | []string   | bundled `hive-tmux -b`         | Commands run after batch session creation          |
| `recycle`       | []string   | git fetch/checkout/reset/clean | Commands run when recycling a session             |
| `commands`      | []string   | `[]`                           | Setup commands run after clone                    |
| `copy`          | []string   | `[]`                           | Glob patterns for files to copy from parent repo  |
| `max_recycled`  | *int       | `5`                            | Maximum recycled sessions to keep (0 = unlimited) |

## Template Variables

Commands support Go templates with `{{ .Variable }}` syntax and `{{ .Variable | shq }}` for shell-safe quoting.

| Context               | Variables                                                           |
| --------------------- | ------------------------------------------------------------------- |
| `rules[].spawn`       | `.Path`, `.Name`, `.Slug`, `.ContextDir`, `.Owner`, `.Repo`         |
| `rules[].batch_spawn` | Same as spawn, plus `.Prompt`                                       |
| `rules[].recycle`     | `.DefaultBranch`                                                    |
| `usercommands.*.sh`   | `.Path`, `.Name`, `.Remote`, `.ID`, `.Tool`, `.TmuxWindow`, `.Args`, `.Form.*` |

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
