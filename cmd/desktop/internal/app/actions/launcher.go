package actions

import (
	"fmt"
	"strings"

	"github.com/colonyops/hive/cmd/desktop/internal/app/icons"
)

// Launcher is a named command that opens in the pop-up terminal (ADR ephemeral-popup-terminals),
// reached from the command palette or a keybinding of its own.
//
// It shares actions.yml with the action catalog — one file, one loader, one
// watcher, one place a user edits — but it is not an action and does not
// pretend to be one: it never reaches the dispatcher, has no executor, no exit
// status and no job, and none of the action envelope (`targets`, `applies_to`,
// `show_in_detail`, `inputs`) means anything to it (ADR launchers-are-their-own-list-in-actions-yml).
//
// Neither Command nor Cwd is a template. There is no triggering item to render
// over, and none is needed: the command runs through a login shell in the
// working directory, so a user's PATH, aliases, functions and $PWD already
// resolve it.
type Launcher struct {
	// ID names the launcher, and is what its bindable command id is built from
	// — `launcher.<id>` in settings.yaml's keybindings.
	ID string `json:"id" yaml:"id"`
	// Label is the human-readable name shown in the command palette.
	Label string `json:"label" yaml:"label"`
	// Command is the shell command line the terminal opens into.
	Command string `json:"command" yaml:"command"`
	// Cwd pins the launcher to one directory, and is what makes a launcher
	// reachable with no session open. Empty means the checkout of the session on
	// screen — the launcher follows the session you are in, and is offered only
	// while you are in one (ADR quick-terminal-launchers-are-session-scoped).
	Cwd string `json:"cwd,omitempty" yaml:"cwd,omitempty"`
	// Icon is the glyph the command palette shows, from the launcher set in
	// internal/app/icons. Empty means the terminal glyph.
	Icon string `json:"icon,omitempty" yaml:"icon,omitempty"`
}

func (l Launcher) Validate() error {
	if l.ID == "" {
		return fmt.Errorf("launcher: id is required")
	}
	if !validSlug(l.ID) {
		return fmt.Errorf("launcher %q: id is not a valid slug (lowercase letters, digits, hyphens, starting with a letter or digit, max %d chars)", l.ID, maxSlugLen)
	}
	if l.Label == "" {
		return fmt.Errorf("launcher %q: label is required", l.ID)
	}
	if strings.TrimSpace(l.Command) == "" {
		return fmt.Errorf("launcher %q: command is required", l.ID)
	}
	if !icons.ValidLauncher(l.Icon) {
		return fmt.Errorf("launcher %q: unknown icon %q", l.ID, l.Icon)
	}
	return nil
}
