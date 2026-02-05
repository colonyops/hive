---
name: hive:config
description: Configure hive for your workflow - rules, spawn commands, keybindings, and terminal integration. Use when setting up new repos or customizing session behavior.
compatibility: claude, opencode
---

# Config - Configure Hive for Your Workflow

Set up and customize hive to match your development workflow with rules, spawn commands, keybindings, and terminal integration.

## When to Use

Use this skill when:
- Setting up hive for the first time
- Configuring repository-specific behavior
- Customizing spawn commands for your terminal
- Adding keybindings or user commands
- Enabling tmux integration
- Creating automation workflows

**Common triggers:**
- "configure hive"
- "setup hive for my workflow"
- "customize session spawn"
- "add tmux integration"
- "create custom keybindings"

## Configuration File

**Location:** `~/.config/hive/config.yaml`

**Format:** YAML

**Check current config:**
```bash
cat ~/.config/hive/config.yaml
```

**Edit config:**
```bash
$EDITOR ~/.config/hive/config.yaml
```

## Configuration Structure

### Minimal Config

```yaml
version: 0.2.4

rules:
  - pattern: ""  # Matches all repos
    spawn:
      - 'wezterm cli spawn --cwd "{{ .Path }}" -- claude'
```

### Complete Config Example

```yaml
version: 0.2.4

# Directories to scan for repos (enables 'n' key in TUI)
repo_dirs:
  - ~/code/repos
  - ~/projects

# Terminal integration
integrations:
  terminal:
    enabled: [tmux]
    poll_interval: 500ms

# TUI settings
tui:
  refresh_interval: 10s
  preview_enabled: true

# Repository-specific rules
rules:
  # Default rule (empty pattern matches all)
  - pattern: ""
    max_recycled: 5
    spawn:
      - tmux new-session -d -s "{{ .Name }}" -c "{{ .Path }}"
    commands:
      - hive ctx init

  # Work repos
  - pattern: ".*github\\.com/my-org/.*"
    spawn:
      - 'wezterm cli spawn --cwd "{{ .Path }}" -- aider'
    commands:
      - npm install
    copy:
      - .envrc

# User commands
usercommands:
  review:
    sh: "send-claude {{ .Name }} /review"
    help: "Send /review to Claude"
    silent: true

# Keybindings
keybindings:
  r:
    cmd: Recycle
    confirm: "Recycle session?"
  d:
    cmd: Delete
```

## Rules System

Rules match repository URLs with regex patterns and define behavior for those repos.

### Rule Structure

```yaml
rules:
  - pattern: ".*github\\.com/org-name/.*"  # Regex pattern
    max_recycled: 3                         # Max recycled sessions
    spawn: []                               # Commands after creation
    batch_spawn: []                         # Commands for batch creation
    recycle: []                             # Commands when recycling
    commands: []                            # Setup commands after clone
    copy: []                                # Files to copy from original
```

### Pattern Matching

Patterns are regex strings matched against the git remote URL.

**Examples:**
```yaml
# Match all repos
- pattern: ""

# Match specific org
- pattern: ".*github\\.com/my-org/.*"

# Match specific repo
- pattern: ".*github\\.com/my-org/my-repo"

# Match by protocol
- pattern: "git@github\\.com:.*"
- pattern: "https://.*"

# Match multiple orgs
- pattern: ".*(my-org|other-org)/.*"
```

**Rule precedence:** First matching rule wins. Put specific patterns before general ones.

### Spawn Commands

Commands run after creating a new session.

**spawn** - Interactive session creation (via TUI or `hive new`)
**batch_spawn** - Background session creation (via `hive batch`)

```yaml
rules:
  - pattern: ""
    spawn:
      # Open tmux session
      - tmux new-session -d -s "{{ .Name }}" -c "{{ .Path }}"

    batch_spawn:
      # Background tmux with initial prompt
      - tmux new-session -d -s "{{ .Name }}" -c "{{ .Path }}" "claude '{{ .Prompt }}'"
```

### Recycle Commands

Commands run when recycling a session (returning to clean state).

**Default recycle behavior:**
```yaml
recycle:
  - git fetch origin
  - git checkout -f {{ .DefaultBranch }}
  - git reset --hard origin/{{ .DefaultBranch }}
  - git clean -fd
```

**Override for custom workflow:**
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

### Setup Commands

Commands run after cloning the repository.

```yaml
rules:
  - pattern: ".*my-project.*"
    commands:
      - hive ctx init           # Create .hive symlink
      - npm install             # Install dependencies
      - cp .env.example .env    # Setup environment
      - make setup              # Run project setup
```

### Copy Files

Copy files from original repo to session.

```yaml
rules:
  - pattern: ""
    copy:
      - .envrc          # Direnv config
      - .env.local      # Local environment
      - config/*.yaml   # Config files (glob patterns supported)
```

## Template Variables

Commands support Go templates with `{{ .Variable }}` syntax.

### Available Variables

| Context | Variables |
|---------|-----------|
| `spawn`, `batch_spawn` | `.Path`, `.Name`, `.Slug`, `.ContextDir`, `.Owner`, `.Repo` |
| `batch_spawn` (additional) | `.Prompt` |
| `recycle` | `.DefaultBranch` |
| `usercommands.*.sh` | `.Path`, `.Name`, `.Remote`, `.ID`, `.Args` |

### Variable Descriptions

**`.Path`** - Absolute path to session directory
- Example: `/Users/name/.local/share/hive/repos/myrepo-abc123`

**`.Name`** - Session name (repo-sessionid format)
- Example: `myrepo-abc123`

**`.Slug`** - Repository slug (owner/repo)
- Example: `my-org/myrepo`

**`.ContextDir`** - Shared context directory path
- Example: `/Users/name/.local/share/hive/context/my-org/myrepo`

**`.Owner`** - Repository owner
- Example: `my-org`

**`.Repo`** - Repository name
- Example: `myrepo`

**`.Prompt`** - Initial prompt (batch_spawn only)
- Example: `Implement user authentication`

**`.DefaultBranch`** - Default branch name (recycle only)
- Example: `main` or `master`

**`.ID`** - Session ID (usercommands only)
- Example: `abc123`

**`.Remote`** - Git remote URL (usercommands only)
- Example: `git@github.com:my-org/myrepo.git`

**`.Args`** - Command arguments array (usercommands only)
- Example: `["arg1", "arg2"]`

### Shell Quoting

Use `{{ .Variable | shq }}` to safely quote variables for shell commands:

```yaml
spawn:
  - echo {{ .Name | shq }}          # Safe: handles spaces/quotes
  - 'claude "{{ .Prompt | shq }}"'  # Safe: nested quotes
```

## Terminal Integration

### Tmux Integration

Enable real-time status monitoring of tmux panes:

```yaml
integrations:
  terminal:
    enabled: [tmux]
    poll_interval: 500ms
    preview_window_matcher: ["claude", "aider", "codex"]
```

**Status indicators:**
- `[●]` Green - Agent actively working
- `[!]` Yellow - Agent needs approval
- `[>]` Cyan - Agent ready for input
- `[?]` Dim - Terminal not found
- `[○]` Gray - Session recycled

**Preview sidebar:**
Press `v` in TUI to toggle tmux pane preview.

### Spawn Command Patterns

#### WezTerm

```yaml
spawn:
  - 'wezterm cli spawn --cwd "{{ .Path }}" -- claude'
```

#### Tmux (New Session)

```yaml
spawn:
  - tmux new-session -d -s "{{ .Name }}" -c "{{ .Path }}" claude
```

#### Tmux (Create or Switch)

```yaml
spawn:
  - tmux has-session -t "{{ .Name }}" 2>/dev/null && tmux switch-client -t "{{ .Name }}" || tmux new-session -d -s "{{ .Name }}" -c "{{ .Path }}" claude
```

#### Kitty

```yaml
spawn:
  - 'kitty @ launch --cwd "{{ .Path }}" --type tab claude'
```

#### Alacritty

```yaml
spawn:
  - 'alacritty --working-directory "{{ .Path }}" -e claude &'
```

#### iTerm2 (macOS)

```yaml
spawn:
  - osascript -e 'tell application "iTerm" to create window with default profile command "cd {{ .Path | shq }} && claude"'
```

## User Commands

Define custom commands accessible via `:` command palette or keybindings.

### Command Structure

```yaml
usercommands:
  name:
    sh: "command template"     # Shell command
    help: "Description"        # Help text
    confirm: "Are you sure?"   # Optional confirmation
    silent: true               # Skip loading popup
    exit: "true"               # Exit TUI after command
```

### System Default Commands

Hive provides built-in commands you can reference:

| Name | Description |
|------|-------------|
| `Recycle` | Reset session to clean state |
| `Delete` | Delete session completely |

### Example Commands

```yaml
usercommands:
  # Send slash command to claude
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

  # Send message to inbox
  msg:
    sh: 'hive msg pub -t agent.{{ .ID }}.inbox "{{ range .Args }}{{ . }} {{ end }}"'
    help: "Send message to session inbox"

  # Run tests
  test:
    sh: "cd {{ .Path }} && npm test"
    help: "Run test suite"

  # Git worktree creation
  worktree:
    sh: 'cd {{ .Path }} && git worktree add ../{{ .Repo }}-{{ index .Args 0 }} {{ index .Args 0 }}'
    help: "Create git worktree (usage: :worktree branch-name)"
```

### Arguments Support

Access arguments via `.Args` template variable:

```yaml
usercommands:
  checkout:
    sh: "cd {{ .Path }} && git checkout {{ index .Args 0 }}"
    help: "Checkout branch (usage: :checkout branch-name)"
```

**Usage:** `:checkout feature-x` → checks out `feature-x`

### Exit Conditions

Control when TUI exits after command:

```yaml
usercommands:
  attach:
    sh: "tmux attach -t {{ .Name }}"
    exit: "$HIVE_POPUP"  # Exit if HIVE_POPUP env var is true
```

**Values:**
- `"true"` - Always exit
- `"false"` - Never exit
- `"$ENV_VAR"` - Exit if environment variable is set to "true"

## Keybindings

Map keys to user commands in the TUI.

### Keybinding Structure

```yaml
keybindings:
  key:
    cmd: command-name         # Command to execute
    help: "Override help"     # Override command's help text
    confirm: "Are you sure?"  # Override command's confirmation
```

### Example Keybindings

```yaml
keybindings:
  # System defaults
  r:
    cmd: Recycle
    confirm: "Recycle this session?"
  d:
    cmd: Delete

  # Custom commands
  o:
    cmd: vscode
  t:
    cmd: tidy
  enter:
    cmd: attach
  ctrl+t:
    cmd: test
```

### Special Keys

**Single keys:** `a-z`, `0-9`, `-`, `=`, `[`, `]`, etc.
**Modified keys:** `ctrl+a`, `alt+x`, `shift+f1`
**Function keys:** `f1`, `f2`, ..., `f12`
**Special:** `enter`, `space`, `tab`, `esc`, `backspace`

### Reserved Keys

These keys are used by hive and cannot be bound:

- `?` - Help screen
- `:` - Command palette (if usercommands configured)
- `v` - Toggle preview (if tmux enabled)
- `j/k/↑/↓` - Navigation
- `/` - Filter
- `n` - New session (if repo_dirs configured)

## Common Workflow Examples

### Basic Setup (All Repos)

```yaml
rules:
  - pattern: ""
    spawn:
      - tmux new-session -d -s "{{ .Name }}" -c "{{ .Path }}" claude
    commands:
      - hive ctx init
```

### Organization-Specific Setup

```yaml
rules:
  # Work repos with setup script
  - pattern: ".*github\\.com/my-company/.*"
    spawn:
      - tmux new-session -d -s "{{ .Name }}" -c "{{ .Path }}" claude
    commands:
      - hive ctx init
      - make dev-setup
      - npm install
    copy:
      - .envrc

  # Personal repos (simple)
  - pattern: ".*github\\.com/my-username/.*"
    spawn:
      - tmux new-session -d -s "{{ .Name }}" -c "{{ .Path }}" claude
    commands:
      - hive ctx init
```

### Monorepo with Multiple Tools

```yaml
rules:
  - pattern: ".*monorepo.*"
    spawn:
      # Create tmux session with multiple panes
      - tmux new-session -d -s "{{ .Name }}" -c "{{ .Path }}"
      - tmux split-window -h -t "{{ .Name }}" -c "{{ .Path }}"
      - tmux send-keys -t "{{ .Name }}:0.0" "claude" Enter
      - tmux send-keys -t "{{ .Name }}:0.1" "npm run dev" Enter
    commands:
      - npm install
      - npm run build
```

### Git Worktree Pattern

```yaml
rules:
  - pattern: ""
    spawn:
      - tmux new-session -d -s "{{ .Name }}" -c "{{ .Path }}" claude
    commands:
      - hive ctx init
      - bd init --stealth || true  # Initialize beads tracking

usercommands:
  worktree:
    sh: 'cd {{ .Path }} && git worktree add ../{{ .Repo }}-{{ index .Args 0 }} {{ index .Args 0 }}'
    help: "Create worktree from branch"
```

### Terminal Emulator Matrix

```yaml
# WezTerm
rules:
  - pattern: ""
    spawn:
      - 'wezterm cli spawn --cwd "{{ .Path }}" -- claude'

# Tmux with custom layout script
rules:
  - pattern: ""
    spawn:
      - ~/.config/tmux/layouts/hive.sh "{{ .Name }}" "{{ .Path }}"

# Kitty with tab
rules:
  - pattern: ""
    spawn:
      - 'kitty @ launch --type tab --cwd "{{ .Path }}" claude'
```

## Best Practices

### Rule Organization

1. **Specific before general:** More specific patterns should come first
2. **Test patterns:** Use `echo "url" | grep -E "pattern"` to test regex
3. **Document patterns:** Add comments explaining complex regex

```yaml
rules:
  # Critical production repos
  - pattern: ".*github\\.com/company/prod-.*"
    max_recycled: 1  # Keep clean

  # Regular work repos
  - pattern: ".*github\\.com/company/.*"
    max_recycled: 3

  # Catch-all
  - pattern: ""
    max_recycled: 5
```

### Command Safety

1. **Quote variables:** Use `{{ .Variable | shq }}` for shell safety
2. **Handle failures:** Use `|| true` for optional commands
3. **Test commands:** Run manually before adding to config

```yaml
commands:
  - npm install || echo "npm not found"  # Handle missing npm
  - test -f .envrc && direnv allow       # Conditional execution
```

### Template Variables

1. **Use `.Path` for working directory:** `cd {{ .Path }}`
2. **Use `.Name` for session references:** `tmux attach -t {{ .Name }}`
3. **Use `.Prompt` for batch initialization:** Only available in `batch_spawn`

## Troubleshooting

### Config Not Loading

**Problem:** Changes to config.yaml not taking effect

**Solutions:**
- Check config location: `~/.config/hive/config.yaml`
- Verify YAML syntax: Use `yamllint` or online validator
- Check for error messages: Run `hive ls` and look for warnings
- Restart hive TUI: Exit and relaunch

### Pattern Not Matching

**Problem:** Rule not applying to expected repos

**Solutions:**
- Test regex: `echo "git@github.com:org/repo" | grep -E "pattern"`
- Check remote URL: `cd repo && git remote -v`
- Escape special chars: Use `\\.` for literal dots
- Check pattern order: More specific patterns must come first

### Spawn Command Fails

**Problem:** Terminal doesn't open or crashes

**Solutions:**
- Test command manually: Run with substituted variables
- Check template syntax: Verify `{{ .Variable }}` format
- Quote paths: Use `{{ .Path | shq }}` for spaces
- Check executable exists: `which tmux`, `which wezterm`, etc.

### Template Variable Empty

**Problem:** Variable expands to empty string

**Solutions:**
- Check variable availability: Some only work in specific contexts
- Verify spelling: Variable names are case-sensitive
- Check session metadata: `hive ls` shows available data
- Use default values: `{{ .Variable | default "fallback" }}`

## Migration

Check your config version and migrate if needed:

```bash
hive doc migrate
```

Current version: **0.2.4**

### Key Changes in 0.2.4

- **Keybindings:** Must reference commands via `cmd` field
- **Built-in commands:** `Recycle` and `Delete` are now user commands
- **Actions removed:** Use `cmd: Recycle` instead of `action: recycle`

## Related Skills

- `/hive:inbox` - Check inter-agent messages
- `/hive:publish` - Send messages between sessions
- `/hive:session-info` - Get current session details

## Tips

**Start Simple:**
- Begin with minimal config
- Add complexity as needed
- Test each change before adding more

**Use Version Control:**
- Keep config in dotfiles repo
- Document custom patterns
- Share configs across machines

**Leverage Templates:**
- Create reusable command patterns
- Use variables for flexibility
- Quote safely with `| shq`

**Optimize for Your Workflow:**
- Map frequent actions to single keys
- Create commands for common tasks
- Use spawn commands to set up environment exactly how you want it
