package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/urfave/cli/v3"
)

type hookEntry struct {
	Matcher string     `json:"matcher,omitempty"`
	Hooks   []hookItem `json:"hooks"`
}

type hookItem struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Async   bool   `json:"async"`
}

// hiveHookCommand is the command string installed into Claude's settings.
const hiveHookCommand = "hive hook-handler"

// claudeHookEvents lists the events to install, with optional matchers.
var claudeHookEvents = []struct {
	Event   string
	Matcher string
}{
	{Event: "SessionStart"},
	{Event: "UserPromptSubmit"},
	{Event: "Stop"},
	{Event: "PermissionRequest"},
	{Event: "SessionEnd"},
	{Event: "Notification", Matcher: "permission_prompt|elicitation_dialog"},
}

// InstallCmd handles the `hive install` subcommand.
type InstallCmd struct {
	flags *Flags
}

// NewInstallCmd creates a new install command.
func NewInstallCmd(flags *Flags) *InstallCmd {
	return &InstallCmd{flags: flags}
}

// Register adds the install command to the application.
func (cmd *InstallCmd) Register(app *cli.Command) *cli.Command {
	app.Commands = append(app.Commands, &cli.Command{
		Name:  "install",
		Usage: "Install hive integrations",
		Commands: []*cli.Command{
			{
				Name:   "claude",
				Usage:  "Install hive hooks into ~/.claude/settings.json",
				Action: cmd.runClaude,
			},
		},
	})
	return app
}

func (cmd *InstallCmd) runClaude(_ context.Context, _ *cli.Command) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	// Resolve any symlink so Rename writes to the real file, not a new regular file
	// (important for users managing ~/.claude/settings.json via dotfile managers).
	if resolved, err := filepath.EvalSymlinks(settingsPath); err == nil {
		settingsPath = resolved
	}

	// Read existing settings, preserving all unknown fields.
	raw := make(map[string]json.RawMessage)
	if data, err := os.ReadFile(settingsPath); err == nil {
		if err := json.Unmarshal(data, &raw); err != nil {
			return fmt.Errorf("parse %s: %w", settingsPath, err)
		}
	}

	// Decode existing hooks (may be absent).
	var hooks map[string][]hookEntry
	if v, ok := raw["hooks"]; ok {
		if err := json.Unmarshal(v, &hooks); err != nil {
			return fmt.Errorf("parse existing hooks: %w", err)
		}
	}
	if hooks == nil {
		hooks = make(map[string][]hookEntry)
	}

	installed := 0
	for _, ev := range claudeHookEvents {
		if addHookEntry(hooks, ev.Event, ev.Matcher) {
			installed++
		}
	}

	if installed == 0 {
		fmt.Println("hive hooks already installed in", settingsPath)
		return nil
	}

	hooksJSON, err := json.Marshal(hooks)
	if err != nil {
		return fmt.Errorf("encode hooks: %w", err)
	}
	raw["hooks"] = hooksJSON

	// Write atomically via a temp file.
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o700); err != nil {
		return fmt.Errorf("create .claude dir: %w", err)
	}
	tmp := settingsPath + ".tmp"
	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := os.Rename(tmp, settingsPath); err != nil {
		return fmt.Errorf("replace settings file: %w", err)
	}

	fmt.Printf("installed %d hive hook(s) into %s\n", installed, settingsPath)

	if _, err := exec.LookPath("hive"); err != nil {
		fmt.Fprintln(os.Stderr, "warning: 'hive' not found in PATH — hooks will fail silently until hive is in PATH")
	}

	return nil
}

// addHookEntry adds a hive hook-handler entry for the given event if not already present.
// Returns true if an entry was added.
func addHookEntry(hooks map[string][]hookEntry, event, matcher string) bool {
	entries := hooks[event]
	for _, entry := range entries {
		if entry.Matcher == matcher {
			for _, item := range entry.Hooks {
				if item.Command == hiveHookCommand {
					return false // already installed
				}
			}
		}
	}

	hooks[event] = append(hooks[event], hookEntry{
		Matcher: matcher,
		Hooks: []hookItem{
			{Type: "command", Command: hiveHookCommand, Async: true},
		},
	})
	return true
}
