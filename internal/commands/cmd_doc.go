package commands

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/hay-kot/hive/internal/core/config"
	"github.com/urfave/cli/v3"
)

type DocCmd struct {
	flags *Flags
	all   bool
}

func NewDocCmd(flags *Flags) *DocCmd {
	return &DocCmd{flags: flags}
}

func (cmd *DocCmd) Register(app *cli.Command) *cli.Command {
	app.Commands = append(app.Commands, &cli.Command{
		Name:  "doc",
		Usage: "Documentation and migration guides",
		Description: `Access documentation and migration guides for hive.

Use 'hive doc migrate' to see configuration migration information.
Use 'hive doc messaging' to see inter-agent messaging conventions.`,
		Commands: []*cli.Command{
			cmd.migrateCmd(),
			cmd.messagingCmd(),
		},
	})
	return app
}

func (cmd *DocCmd) migrateCmd() *cli.Command {
	return &cli.Command{
		Name:  "migrate",
		Usage: "Show configuration migration guide",
		Description: `Outputs migration information for config changes between versions.

By default, only shows migrations needed for your current config version.
Use --all to show all migrations.`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "all",
				Usage:       "show all migrations, not just those needed for your config",
				Destination: &cmd.all,
			},
		},
		Action: cmd.runMigrate,
	}
}

func (cmd *DocCmd) runMigrate(_ context.Context, c *cli.Command) error {
	w := c.Root().Writer
	configVersion := cmd.flags.Config.Version
	printMigrationGuide(w, configVersion, cmd.all)
	return nil
}

func (cmd *DocCmd) messagingCmd() *cli.Command {
	return &cli.Command{
		Name:  "messaging",
		Usage: "Show inter-agent messaging conventions for LLMs",
		Description: `Outputs documentation for LLMs on hive messaging conventions.

This guide explains how AI agents can communicate with each other
using hive's messaging system.`,
		Action: cmd.runMessaging,
	}
}

func (cmd *DocCmd) runMessaging(_ context.Context, c *cli.Command) error {
	w := c.Root().Writer
	printMessagingGuide(w)
	return nil
}

func printMessagingGuide(w io.Writer) {
	guide := `# Hive Inter-Agent Messaging Guide

## Identity

Your session ID is available via:
` + "```bash" + `
hive session info
` + "```" + `

Your inbox topic follows the convention: ` + "`agent.{session-id}.inbox`" + `

## Checking Your Inbox

Read unread messages only (since last check):
` + "```bash" + `
hive msg sub -t agent.{id}.inbox --new
` + "```" + `

Read all messages:
` + "```bash" + `
hive msg sub -t agent.{id}.inbox
` + "```" + `

## Sending to Another Agent

First, discover active agents:
` + "```bash" + `
hive ls --json
` + "```" + `

Then send to their inbox:
` + "```bash" + `
hive msg pub -t agent.{their-id}.inbox "your message"
` + "```" + `

## Messaging Conventions

- **Check inbox on startup** for handoffs from other agents
- **Use inbox for direct communication** between specific agents
- **Use custom topics for broadcast** patterns (e.g., "build.status", "test.results")
- **Include context** in messages: what you're working on, what you need
- **Include your session ID** for reply routing

## Message Format Suggestions

**For structured data:** JSON
` + "```json" + `
{"from": "abc123", "type": "handoff", "task": "review PR #42", "context": "..."}
` + "```" + `

**For human-readable handoffs:** Markdown
` + "```markdown" + `
## Handoff from abc123

I completed the authentication refactor. Tests are passing.

### Remaining work:
- Update the README
- Add integration tests
` + "```" + `

## Topic Naming Conventions

| Pattern | Use Case |
|---------|----------|
| ` + "`agent.{id}.inbox`" + ` | Direct messages to a specific agent |
| ` + "`build.{repo}`" + ` | Build status updates |
| ` + "`test.results`" + ` | Test run notifications |
| ` + "`deploy.{env}`" + ` | Deployment events |

## Quick Reference

| Command | Description |
|---------|-------------|
| ` + "`hive session info`" + ` | Get your session ID |
| ` + "`hive ls --json`" + ` | List agents with inbox info |
| ` + "`hive msg sub -t TOPIC`" + ` | Subscribe to a topic |
| ` + "`hive msg sub -t TOPIC --new`" + ` | Subscribe to unread only |
| ` + "`hive msg pub -t TOPIC MSG`" + ` | Publish a message |
| ` + "`hive msg topic --new`" + ` | Generate a new topic ID |
`
	_, _ = fmt.Fprintln(w, guide)
}

// Migration represents a breaking change that requires user action.
type Migration struct {
	Version     string
	Title       string
	Description string
	Migration   string
	Before      string
	After       string
}

var migrations = []Migration{
	{
		Version:     "0.2.5",
		Title:       "Multi-window tmux sessions with TmuxWindow template variable",
		Description: "When a tmux session has multiple agent windows (matched by preview_window_matcher), each window now appears as its own selectable tree item with independent status and preview. The new {{ .TmuxWindow }} template variable is available in user commands and resolves to the selected window name. If you use a custom script (like hive.sh) with an enter keybinding, you must update the template to pass the window name so that pressing enter on a window sub-item focuses the correct window.",
		Migration: `1. Add {{ .TmuxWindow }} to your tmux-open (or attach/enter) user command
2. Update your spawn script to accept a window target flag (e.g., -w)
3. Handle the window argument in your script to switch to the correct window`,
		Before: `# config.yaml (old)
usercommands:
  tmux-open:
    sh: ~/.config/tmux/layouts/hive.sh "{{ .Name }}" "{{ .Path }}"
    help: "open/create tmux"
    exit: $HIVE_POPUP
    silent: true`,
		After: `# config.yaml (new)
usercommands:
  tmux-open:
    sh: ~/.config/tmux/layouts/hive.sh -w {{ .TmuxWindow | shq }} {{ .Name | shq }} {{ .Path | shq }}
    help: "open/create tmux"
    exit: $HIVE_POPUP
    silent: true

# The -w flag tells your script which window to focus.
# When a window sub-item is selected, .TmuxWindow = that window's name.
# When a session row is selected, .TmuxWindow = best disambiguated window.
# When empty (single window), the -w "" is ignored by the script.`,
	},
	{
		Version:     "0.2.4",
		Title:       "Tmux config moved to top-level",
		Description: "Terminal integration config has been moved from integrations.terminal to a top-level tmux key. This simplifies the config structure since tmux is now a core feature rather than an optional integration.",
		Migration: `Update your config.yaml to move integrations.terminal settings to the tmux key.
All three settings (enabled, poll_interval, preview_window_matcher) are now directly under tmux.`,
		Before: `# config.yaml (old - 0.2.3)
integrations:
  terminal:
    enabled: [tmux]
    poll_interval: 500ms
    preview_window_matcher: [claude, aider, codex]`,
		After: `# config.yaml (new - 0.2.4)
tmux:
  enabled: [tmux]
  poll_interval: 500ms
  preview_window_matcher: [claude, aider, codex]`,
	},
	{
		Version:     "0.2.4",
		Title:       "Keybindings now reference commands",
		Description: "Keybindings have been simplified to only reference UserCommands. Built-in actions (recycle, delete) are now provided as default UserCommands named 'Recycle' and 'Delete'. All keybindings must have a 'cmd' field referencing a UserCommand.",
		Migration: `1. Move any 'sh:' commands from keybindings to usercommands
2. Replace 'action: recycle' with 'cmd: Recycle'
3. Replace 'action: delete' with 'cmd: Delete'
4. For shell keybindings, create a usercommand and reference it`,
		Before: `# config.yaml (old - 0.2.3)
keybindings:
  r:
    action: recycle
    confirm: "Recycle?"
  d:
    action: delete
  o:
    sh: "code {{ .Path }}"
    help: "open in vscode"`,
		After: `# config.yaml (new - 0.2.4)
usercommands:
  vscode:
    sh: "code {{ .Path }}"
    help: "open in vscode"

keybindings:
  r:
    cmd: Recycle        # System default command
    confirm: "Recycle?" # Override default confirm
  d:
    cmd: Delete         # System default command
  o:
    cmd: vscode         # User-defined command`,
	},
	{
		Version:     "0.2.3",
		Title:       "User commands and command palette",
		Description: "The TUI now supports user-defined commands accessible via a vim-style command palette (press `:` key). Commands can accept arguments and include confirmation prompts, exit conditions, and more.",
		Migration:   "No action required. Add usercommands to your config to enable the command palette.",
		After: `# config.yaml
usercommands:
  review:
    sh: "send-claude {{ .Name }} /review"
    help: "Send /review to Claude session"
    silent: true
  tidy:
    sh: "send-claude {{ .Name }} /tidy"
    help: "Send /tidy to Claude session"
    confirm: "Commit and push changes?"
  msg:
    sh: 'hive msg pub -t agent.{{ .ID }}.inbox "{{ range .Args }}{{ . }} {{ end }}"'
    help: "Send message to session inbox"

# Press : in TUI to open command palette
# Available template variables: .Path, .Name, .Remote, .ID, .Args
# Command options: sh (required), help, confirm, silent, exit`,
	},
	{
		Version:     "0.2.3",
		Title:       "Configurable topic prefix for msg topic command",
		Description: "The `hive msg topic` command now supports a configurable prefix via config or --prefix flag. The default prefix is \"agent\".",
		Migration:   "No action required. To customize the default prefix, add messaging.topic_prefix to your config.",
		After: `# config.yaml
messaging:
  topic_prefix: agent  # default, generates "agent.x7k2"

# Or use --prefix flag to override:
hive msg topic --prefix task    # generates "task.x7k2"
hive msg topic --prefix ""      # generates "x7k2" (no prefix)`,
	},
	{
		Version:     "0.2.2",
		Title:       "New max_recycled rule setting",
		Description: "Rules can now set max_recycled to limit recycled sessions per repository. Oldest sessions beyond the limit are automatically deleted when recycling.",
		Migration:   "No action required. Default is 5 sessions per repo. Configure via rules with empty pattern as catch-all.",
		After: `# config.yaml
rules:
  # Catch-all sets the default (code default is 5 if not set)
  - pattern: ""
    max_recycled: 5

  # Override for specific repos
  - pattern: "github.com/my-org/large-repo"
    max_recycled: 2  # keep fewer

  # Unlimited for some repos
  - pattern: "github.com/my-org/special-repo"
    max_recycled: 0  # 0 = unlimited`,
	},
	{
		Version:     "0.2.2",
		Title:       "New prune --all flag",
		Description: "The `hive prune` command now respects max_recycled limits by default. Use --all to delete all recycled sessions.",
		Migration:   "If you want the old behavior (delete all recycled), use `hive prune --all`.",
		Before:      `hive prune  # deleted all recycled sessions`,
		After:       `hive prune --all  # same behavior as before`,
	},
	{
		Version:     "0.2.1",
		Title:       "New TUI auto-refresh feature",
		Description: "The TUI sessions view now auto-refreshes every 15 seconds by default. This can be configured or disabled.",
		Migration:   "No action required. To customize, add tui.refresh_interval to your config.",
		After: `# config.yaml
tui:
  refresh_interval: 15s  # default, set to 0 to disable`,
	},
	{
		Version:     "0.2.0",
		Title:       "Removed `--prompt` flag from `hive new`",
		Description: "The `--prompt` flag was removed from `hive new`. Prompts are now only supported in batch mode via `hive batch`.",
		Migration: `- If you were using "hive new --prompt '...'", use "hive batch" instead
- Configure "batch_spawn" in your config to support prompts`,
		Before: `hive new my-session --prompt "Fix the bug"`,
		After:  `echo '{"sessions":[{"name":"my-session","prompt":"Fix the bug"}]}' | hive batch`,
	},
	{
		Version:     "0.2.0",
		Title:       "New `batch_spawn` config option",
		Description: "Added separate spawn commands for batch sessions that support the `{{.Prompt}}` template variable.",
		Migration: `- Add "batch_spawn" to your config if you need prompt support
- The "spawn" command no longer supports the "{{.Prompt}}" template variable`,
		Before: `# config.yaml (old - spawn with prompt)
commands:
  spawn:
    - "wezterm cli spawn --cwd {{.Path}} -- claude --prompt '{{.Prompt}}'"`,
		After: `# config.yaml (new - separate commands)
commands:
  spawn:        # For hive new (no prompt)
    - "wezterm cli spawn --cwd {{.Path}}"
  batch_spawn:  # For hive batch (with prompt)
    - "wezterm cli spawn --cwd {{.Path}} -- claude --prompt '{{.Prompt}}'"`,
	},
	{
		Version:     "0.2.0",
		Title:       "Removed command history",
		Description: "The `hive history` command and history tracking were removed to simplify the codebase.",
		Migration:   "No action needed unless you were using history programmatically.",
	},
}

func printMigrationGuide(w io.Writer, configVersion string, showAll bool) {
	_, _ = fmt.Fprintln(w, "# Hive Configuration Migration Guide")
	_, _ = fmt.Fprintln(w)

	// Show version status
	if configVersion == "" {
		_, _ = fmt.Fprintln(w, "**Config version:** not set")
	} else {
		_, _ = fmt.Fprintf(w, "**Config version:** %s\n", configVersion)
	}
	_, _ = fmt.Fprintf(w, "**Latest version:** %s\n", config.CurrentConfigVersion)
	_, _ = fmt.Fprintln(w)

	if !showAll && configVersion != "" && compareVersions(configVersion, config.CurrentConfigVersion) >= 0 {
		_, _ = fmt.Fprintln(w, "Your config is up to date. No migrations needed.")
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintln(w, "Use --all to see all migrations.")
		return
	}

	// Filter migrations to only those newer than configVersion
	filtered := migrations
	if !showAll && configVersion != "" {
		filtered = nil
		for _, m := range migrations {
			if compareVersions(m.Version, configVersion) > 0 {
				filtered = append(filtered, m)
			}
		}
	}

	if len(filtered) == 0 {
		_, _ = fmt.Fprintln(w, "No migrations to show.")
		return
	}

	// Group migrations by version
	byVersion := make(map[string][]Migration)
	var versions []string
	for _, m := range filtered {
		if _, exists := byVersion[m.Version]; !exists {
			versions = append(versions, m.Version)
		}
		byVersion[m.Version] = append(byVersion[m.Version], m)
	}

	for i, version := range versions {
		if i == 0 && version == config.CurrentConfigVersion {
			_, _ = fmt.Fprintf(w, "## Version %s (Current)\n", version)
		} else {
			_, _ = fmt.Fprintf(w, "## Version %s\n", version)
		}
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintln(w, "### Breaking Changes")
		_, _ = fmt.Fprintln(w)

		for j, m := range byVersion[version] {
			_, _ = fmt.Fprintf(w, "#### %d. %s\n", j+1, m.Title)
			_, _ = fmt.Fprintln(w)
			_, _ = fmt.Fprintf(w, "**What changed:** %s\n", m.Description)
			_, _ = fmt.Fprintln(w)
			_, _ = fmt.Fprintln(w, "**Migration:**")
			for _, line := range strings.Split(m.Migration, "\n") {
				_, _ = fmt.Fprintln(w, line)
			}
			_, _ = fmt.Fprintln(w)

			if m.Before != "" {
				_, _ = fmt.Fprintln(w, "**Before:**")
				_, _ = fmt.Fprintln(w, "```")
				_, _ = fmt.Fprintln(w, m.Before)
				_, _ = fmt.Fprintln(w, "```")
				_, _ = fmt.Fprintln(w)
			}

			if m.After != "" {
				_, _ = fmt.Fprintln(w, "**After:**")
				_, _ = fmt.Fprintln(w, "```")
				_, _ = fmt.Fprintln(w, m.After)
				_, _ = fmt.Fprintln(w, "```")
				_, _ = fmt.Fprintln(w)
			}
		}
	}

	if !showAll && configVersion != "" {
		_, _ = fmt.Fprintln(w, "---")
		_, _ = fmt.Fprintf(w, "After migrating, update your config version to: %s\n", config.CurrentConfigVersion)
	}
}

// compareVersions compares two semantic versions.
// Returns -1 if a < b, 0 if a == b, 1 if a > b.
func compareVersions(a, b string) int {
	aParts := parseVersion(a)
	bParts := parseVersion(b)

	for i := 0; i < 3; i++ {
		if aParts[i] < bParts[i] {
			return -1
		}
		if aParts[i] > bParts[i] {
			return 1
		}
	}
	return 0
}

// parseVersion extracts major, minor, patch from a version string.
// Returns [0,0,0] for invalid versions.
func parseVersion(v string) [3]int {
	var parts [3]int
	segments := strings.Split(v, ".")
	for i := 0; i < len(segments) && i < 3; i++ {
		n, _ := strconv.Atoi(segments[i])
		parts[i] = n
	}
	return parts
}
