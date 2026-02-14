# Spawn Command Patterns

Terminal-specific spawn command examples for hive configuration.

## WezTerm

```yaml
spawn:
  - 'wezterm cli spawn --cwd "{{ .Path }}" -- claude'
```

## Tmux (New Session)

```yaml
spawn:
  - tmux new-session -d -s "{{ .Name }}" -c "{{ .Path }}" claude
```

## Tmux (Create or Switch)

```yaml
spawn:
  - tmux has-session -t "{{ .Name }}" 2>/dev/null && tmux switch-client -t "{{ .Name }}" || tmux new-session -d -s "{{ .Name }}" -c "{{ .Path }}" claude
```

## Tmux with Multiple Panes

```yaml
spawn:
  - tmux new-session -d -s "{{ .Name }}" -c "{{ .Path }}"
  - tmux split-window -h -t "{{ .Name }}" -c "{{ .Path }}"
  - tmux send-keys -t "{{ .Name }}:0.0" "claude" Enter
  - tmux send-keys -t "{{ .Name }}:0.1" "npm run dev" Enter
```

## Tmux with Custom Layout Script

```yaml
spawn:
  - ~/.config/tmux/layouts/hive.sh {{ .Name | shq }} {{ .Path | shq }}
```

## Kitty

```yaml
spawn:
  - 'kitty @ launch --cwd "{{ .Path }}" --type tab claude'
```

## Alacritty

```yaml
spawn:
  - 'alacritty --working-directory "{{ .Path }}" -e claude &'
```

## iTerm2 (macOS)

```yaml
spawn:
  - osascript -e 'tell application "iTerm" to create window with default profile command "cd {{ .Path | shq }} && claude"'
```

## Batch Spawn (Background Sessions)

```yaml
batch_spawn:
  - tmux new-session -d -s "{{ .Name }}" -c "{{ .Path }}" "claude '{{ .Prompt }}'"
```

## Template Variable Reference

- **`.Path`** - Absolute path to session directory
- **`.Name`** - Session name (repo-sessionid format)
- **`.Slug`** - Repository slug (owner/repo)
- **`.ContextDir`** - Shared context directory path
- **`.Owner`** - Repository owner
- **`.Repo`** - Repository name
- **`.Prompt`** - Initial prompt (batch_spawn only)
- **`.DefaultBranch`** - Default branch name (recycle only)
- **`.ID`** - Session ID (usercommands only)
- **`.Remote`** - Git remote URL (usercommands only)
- **`.TmuxWindow`** - Tmux window name (usercommands only)
- **`.Args`** - Command arguments array (usercommands only)
