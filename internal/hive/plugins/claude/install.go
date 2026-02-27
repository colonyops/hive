package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/colonyops/hive/internal/core/terminal/hooks"
)

// Shell commands written into Claude Code's hooks configuration.
// They write agent status to StatusFileName in $PWD (the session directory).
const (
	hookPreToolUseCommand = `printf '%s' 'active' > "$PWD/` + hooks.StatusFileName + `"`
	hookStopCommand       = `printf '%s' 'ready' > "$PWD/` + hooks.StatusFileName + `"`
)

// claudeHookEntry is a single hook entry in Claude Code's settings.json.
type claudeHookEntry struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

// claudeHookMatcher groups hook entries under an optional tool matcher.
type claudeHookMatcher struct {
	Matcher string            `json:"matcher"`
	Hooks   []claudeHookEntry `json:"hooks"`
}

// settingsRaw is used for round-tripping unknown fields in settings.json.
type settingsRaw map[string]json.RawMessage

// InstallHooks writes hive's Claude Code hook entries into
// <sessionPath>/.claude/settings.json, merging with any existing settings.
// Existing hooks that are not hive-managed are left untouched.
// Returns an error if the file cannot be read or written.
func InstallHooks(sessionPath string) error {
	settingsPath := filepath.Join(sessionPath, ".claude", "settings.json")

	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		return fmt.Errorf("create .claude dir: %w", err)
	}

	// Load existing settings as a raw map to preserve unknown keys.
	raw := make(settingsRaw)
	if data, err := os.ReadFile(settingsPath); err == nil {
		if jsonErr := json.Unmarshal(data, &raw); jsonErr != nil {
			return fmt.Errorf("parse existing settings.json: %w", jsonErr)
		}
	}

	// Decode the existing "hooks" section (if any).
	hooksMap := make(map[string][]claudeHookMatcher)
	if rawHooks, ok := raw["hooks"]; ok {
		if err := json.Unmarshal(rawHooks, &hooksMap); err != nil {
			return fmt.Errorf("parse existing hooks: %w", err)
		}
	}

	// Merge hive's entries into each event's slice (idempotent).
	mergeHook(hooksMap, "PreToolUse", hookPreToolUseCommand)
	mergeHook(hooksMap, "Stop", hookStopCommand)

	// Re-encode the merged hooks back into the raw map.
	hooksJSON, err := json.Marshal(hooksMap)
	if err != nil {
		return fmt.Errorf("encode hooks: %w", err)
	}
	raw["hooks"] = json.RawMessage(hooksJSON)

	// Write the updated settings.json with indentation.
	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}

	if err := os.WriteFile(settingsPath, append(out, '\n'), 0o644); err != nil {
		return fmt.Errorf("write settings.json: %w", err)
	}

	return nil
}

// mergeHook adds a hive hook entry for the given event if one with the same command
// is not already present. This keeps the operation idempotent.
func mergeHook(hooksMap map[string][]claudeHookMatcher, event, command string) {
	entry := claudeHookEntry{Type: "command", Command: command}

	matchers := hooksMap[event]

	// Check if our command is already present in any matcher.
	for _, m := range matchers {
		if slices.ContainsFunc(m.Hooks, func(e claudeHookEntry) bool {
			return e.Command == command
		}) {
			return // already installed
		}
	}

	// Append a new matcher with our hook entry.
	hooksMap[event] = append(matchers, claudeHookMatcher{
		Matcher: "",
		Hooks:   []claudeHookEntry{entry},
	})
}
