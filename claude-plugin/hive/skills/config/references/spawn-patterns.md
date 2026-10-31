# Spawn Patterns

## Window Configuration (Recommended)

The `windows` field defines tmux windows declaratively. Hive handles session creation, attach/switch, and window focus automatically.

### Basic: Agent + Shell

```yaml
windows:
  - name: "{{ agentWindow }}"
    command: '{{ agentCommand }} {{ agentFlags }}{{- if .Prompt }} {{ .Prompt | shq }}{{ end }}'
    focus: true
  - name: shell
```

### Agent + Test Watcher + Shell

```yaml
windows:
  - name: claude
    command: "claude --model opus"
    focus: true
  - name: tests
    command: "npm run test:watch"
  - name: shell
```

### Multiple Agents

```yaml
windows:
  - name: claude
    command: "claude"
    focus: true
  - name: aider
    command: "aider --model sonnet"
  - name: shell
```

### Custom Working Directory

```yaml
windows:
  - name: claude
    command: "claude"
    dir: "{{ .Path }}/frontend"
    focus: true
  - name: shell
```

### Batch Spawn with Prompt

```yaml
windows:
  - name: "{{ agentWindow }}"
    command: '{{ agentCommand }} {{ agentFlags }}{{- if .Prompt }} {{ .Prompt | shq }}{{ end }}'
    focus: true
  - name: shell
```

## Template Variable Reference

- **`.Path`** - Absolute path to session directory
- **`.Name`** - Session name (repo-sessionid format)
- **`.Slug`** - URL-safe session name (e.g., "my-session-name")
- **`.ContextDir`** - Shared context directory path
- **`.Owner`** - Repository owner
- **`.Repo`** - Repository name
- **`.Prompt`** - Initial prompt (batch sessions only)
- **`.DefaultBranch`** - Default branch name (recycle only)
- **`.ID`** - Session ID (usercommands only)
- **`.Remote`** - Git remote URL (usercommands only)
- **`.TmuxWindow`** - Tmux window name (usercommands only)
- **`.Args`** - Command arguments array (usercommands only)
