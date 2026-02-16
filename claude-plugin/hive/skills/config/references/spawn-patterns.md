# Spawn Patterns

## Window Configuration (Recommended)

The `windows` field defines tmux windows declaratively. Hive handles session creation, attach/switch, and window focus automatically.

### Basic: Agent + Shell

```yaml
windows:
  - name: "{{ agentWindow }}"
    command: '{{ agentCommand }} {{ agentFlags }}'
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
    command: '{{ agentCommand }} {{ agentFlags }}{{- if .Prompt }} {{ .Prompt }}{{ end }}'
    focus: true
  - name: shell
```

## Legacy Spawn Commands

For non-tmux terminals or advanced scripting, use `spawn`/`batch_spawn` with shell commands.

### WezTerm

```yaml
spawn:
  - 'wezterm cli spawn --cwd "{{ .Path }}" -- claude'
```

### Kitty

```yaml
spawn:
  - 'kitty @ launch --cwd "{{ .Path }}" --type tab claude'
```

### Alacritty

```yaml
spawn:
  - 'alacritty --working-directory "{{ .Path }}" -e claude &'
```

### iTerm2 (macOS)

```yaml
spawn:
  - osascript -e 'tell application "iTerm" to create window with default profile command "cd {{ .Path | shq }} && claude"'
```

### Tmux (Shell Commands)

```yaml
spawn:
  - tmux new-session -d -s "{{ .Name }}" -c "{{ .Path }}" claude
```

### Batch Spawn (Background Sessions)

```yaml
batch_spawn:
  - tmux new-session -d -s "{{ .Name }}" -c "{{ .Path }}" "claude '{{ .Prompt }}'"
```

## Template Variable Reference

- **`.Path`** - Absolute path to session directory
- **`.Name`** - Session name (repo-sessionid format)
- **`.Slug`** - URL-safe session name (e.g., "my-session-name")
- **`.ContextDir`** - Shared context directory path
- **`.Owner`** - Repository owner
- **`.Repo`** - Repository name
- **`.Prompt`** - Initial prompt (batch_spawn only)
- **`.DefaultBranch`** - Default branch name (recycle only)
- **`.ID`** - Session ID (usercommands only)
- **`.Remote`** - Git remote URL (usercommands only)
- **`.TmuxWindow`** - Tmux window name (usercommands only)
- **`.Args`** - Command arguments array (usercommands only)
