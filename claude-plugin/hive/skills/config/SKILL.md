---
name: config
description: This skill should be used when the user asks to "configure hive", "setup hive for my workflow", "customize session spawn", "add tmux integration", "create custom keybindings", "add user commands", or needs guidance on hive configuration, rules, spawn commands, terminal integration, or keybindings.
compatibility: claude, opencode
---

# Config - Configure Hive for Your Workflow

Set up and customize hive to match development workflows with rules, spawn commands, keybindings, and terminal integration.

## Configuration File

**Location:** `~/.config/hive/config.yaml`

**Current version:** 0.2.5

```bash
cat ~/.config/hive/config.yaml   # View config
$EDITOR ~/.config/hive/config.yaml  # Edit config
hive doc migrate                  # Check version and migrate
```

## Minimal Config

```yaml
version: 0.2.5

rules:
  - pattern: ""  # Matches all repos
    spawn:
      - 'wezterm cli spawn --cwd "{{ .Path }}" -- claude'
```

## Rules System

Rules match repository URLs with regex patterns and define behavior.

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

**Rule precedence:** First matching rule wins. Put specific patterns before general ones.

### Pattern Examples

```yaml
- pattern: ""                                    # Match all repos
- pattern: ".*github\\.com/my-org/.*"           # Match specific org
- pattern: ".*github\\.com/my-org/my-repo"      # Match specific repo
- pattern: ".*(my-org|other-org)/.*"             # Match multiple orgs
```

## Template Variables

Commands support Go templates with `{{ .Variable }}` syntax.

| Context | Variables |
|---------|-----------|
| `spawn`, `batch_spawn` | `.Path`, `.Name`, `.Slug`, `.ContextDir`, `.Owner`, `.Repo` |
| `batch_spawn` (additional) | `.Prompt` |
| `recycle` | `.DefaultBranch` |
| `usercommands.*.sh` | `.Path`, `.Name`, `.Remote`, `.ID`, `.TmuxWindow`, `.Args` |

Use `{{ .Variable | shq }}` for safe shell quoting.

## Terminal Integration

### Tmux

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

## User Commands

Define custom commands accessible via `:` command palette or keybindings.

```yaml
usercommands:
  name:
    sh: "command template"     # Shell command
    help: "Description"        # Help text
    confirm: "Are you sure?"   # Optional confirmation
    silent: true               # Skip loading popup
    exit: "true"               # Exit TUI after command
```

### Examples

```yaml
usercommands:
  tidy:
    sh: "send-claude {{ .Name }} /tidy"
    help: "Run /tidy in Claude session"
    confirm: "Commit and push changes?"
    silent: true

  vscode:
    sh: "code {{ .Path }}"
    help: "Open session in VS Code"
    silent: true
    exit: "true"

  msg:
    sh: 'hive msg pub -t agent.{{ .ID }}.inbox "{{ range .Args }}{{ . }} {{ end }}"'
    help: "Send message to session inbox"
```

## Keybindings

Map keys to user commands in the TUI.

```yaml
keybindings:
  r:
    cmd: Recycle
    confirm: "Recycle this session?"
  d:
    cmd: Delete
  o:
    cmd: vscode
  enter:
    cmd: attach
```

**Reserved keys:** `?`, `:`, `v`, `j/k/↑/↓`, `/`, `n`

## Additional Resources

For detailed configuration examples, spawn command patterns for different terminals, and migration guides, see:

- **`references/spawn-patterns.md`** - Terminal-specific spawn command examples (tmux, WezTerm, Kitty, iTerm2, Alacritty)
- **`references/workflow-examples.md`** - Complete workflow configurations for common setups
- **`references/troubleshooting.md`** - Common configuration issues and solutions

## Related Skills

- `/hive:inbox` - Check inter-agent messages
- `/hive:publish` - Send messages between sessions
- `/hive:session-info` - Get current session details
