package tui

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"maps"
	"os/exec"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	"github.com/hay-kot/hive/internal/core/config"
	"github.com/hay-kot/hive/internal/core/session"
	"github.com/hay-kot/hive/internal/hive"
	"github.com/hay-kot/hive/pkg/tmpl"
)

// ActionType identifies the kind of action a keybinding triggers.
type ActionType int

const (
	ActionTypeNone ActionType = iota
	ActionTypeRecycle
	ActionTypeDelete
	ActionTypeShell
)

// Action represents a resolved keybinding action ready for execution.
type Action struct {
	Type        ActionType
	Key         string
	Help        string
	Confirm     string // Non-empty if confirmation required
	ShellCmd    string // For shell actions, the rendered command
	SessionID   string
	SessionPath string
	Silent      bool  // Skip loading popup for fast commands
	Exit        bool  // Exit hive after command completes
	Err         error // Non-nil if action resolution failed (e.g., template error)
}

// NeedsConfirm returns true if the action requires user confirmation.
func (a Action) NeedsConfirm() bool {
	return a.Confirm != ""
}

// KeybindingHandler resolves keybindings to actions via UserCommands.
type KeybindingHandler struct {
	keybindings map[string]config.Keybinding
	commands    map[string]config.UserCommand
	service     *hive.Service
}

// NewKeybindingHandler creates a new handler with the given config.
// Commands should be the merged user commands (user config + system defaults).
func NewKeybindingHandler(keybindings map[string]config.Keybinding, commands map[string]config.UserCommand, service *hive.Service) *KeybindingHandler {
	return &KeybindingHandler{
		keybindings: keybindings,
		commands:    commands,
		service:     service,
	}
}

// Resolve attempts to resolve a key press to an action for the given session.
// Recycled sessions only allow delete actions to prevent accidental operations.
func (h *KeybindingHandler) Resolve(key string, sess session.Session) (Action, bool) {
	kb, exists := h.keybindings[key]
	if !exists {
		return Action{}, false
	}

	// Look up the referenced command
	cmd, cmdExists := h.commands[kb.Cmd]
	if !cmdExists {
		// Command reference is invalid - validation should catch this,
		// but log and return gracefully for debugging
		slog.Warn("keybinding references unknown command",
			"key", key,
			"cmd", kb.Cmd)
		return Action{}, false
	}

	// Recycled sessions only allow delete - prevent accidental operations
	if sess.State == session.StateRecycled && cmd.Action != config.ActionDelete {
		return Action{}, false
	}

	// Build action from command, with keybinding overrides
	action := Action{
		Key:         key,
		Help:        kb.Help,
		Confirm:     kb.Confirm,
		SessionID:   sess.ID,
		SessionPath: sess.Path,
		Silent:      cmd.Silent,
		Exit:        cmd.ShouldExit(),
	}

	// Use command values if keybinding doesn't override
	if action.Help == "" {
		action.Help = cmd.Help
	}
	if action.Confirm == "" {
		action.Confirm = cmd.Confirm
	}

	// Resolve action type from command
	if cmd.Action != "" {
		switch cmd.Action {
		case config.ActionRecycle:
			action.Type = ActionTypeRecycle
			if action.Help == "" {
				action.Help = "recycle"
			}
		case config.ActionDelete:
			action.Type = ActionTypeDelete
			if action.Help == "" {
				action.Help = "delete"
			}
		}
		return action, true
	}

	// Shell command
	if cmd.Sh != "" {
		data := struct {
			Path   string
			Remote string
			ID     string
			Name   string
		}{
			Path:   sess.Path,
			Remote: sess.Remote,
			ID:     sess.ID,
			Name:   sess.Name,
		}

		rendered, err := tmpl.Render(cmd.Sh, data)
		if err != nil {
			// Surface template error instead of masking it
			action.Type = ActionTypeShell
			action.Err = fmt.Errorf("template error in command %q: %w", kb.Cmd, err)
			slog.Warn("template rendering failed",
				"key", key,
				"cmd", kb.Cmd,
				"error", err)
			return action, true
		}

		action.Type = ActionTypeShell
		action.ShellCmd = rendered
		return action, true
	}

	return Action{}, false
}

// Execute runs the given action.
// Note: ActionTypeRecycle is not handled here - it uses streaming output
// and is executed directly by the TUI model via Service.RecycleSession.
func (h *KeybindingHandler) Execute(ctx context.Context, action Action) error {
	switch action.Type {
	case ActionTypeDelete:
		return h.service.DeleteSession(ctx, action.SessionID)
	case ActionTypeShell:
		return h.executeShell(ctx, action.ShellCmd)
	default:
		return fmt.Errorf("action type %d not supported by Execute", action.Type)
	}
}

// executeShell runs a shell command and captures stderr for better error messages.
func (h *KeybindingHandler) executeShell(_ context.Context, cmd string) error {
	c := exec.Command("sh", "-c", cmd)
	var stderr bytes.Buffer
	c.Stderr = &stderr

	if err := c.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return fmt.Errorf("command failed: %s", errMsg)
		}
		return fmt.Errorf("command failed: %w", err)
	}
	return nil
}

// HelpEntries returns all configured keybindings for display, sorted by key.
func (h *KeybindingHandler) HelpEntries() []string {
	// Get sorted keys for consistent ordering
	keys := slices.Sorted(maps.Keys(h.keybindings))

	entries := make([]string, 0, len(h.keybindings))
	for _, key := range keys {
		kb := h.keybindings[key]
		help := kb.Help

		// If keybinding doesn't override help, get from command
		if help == "" {
			if cmd, ok := h.commands[kb.Cmd]; ok {
				help = cmd.Help
				if help == "" && cmd.Action != "" {
					help = cmd.Action
				}
			}
		}
		if help == "" {
			help = "unknown"
		}
		entries = append(entries, fmt.Sprintf("[%s] %s", key, help))
	}
	return entries
}

// HelpString returns a formatted help string for all keybindings.
func (h *KeybindingHandler) HelpString() string {
	entries := h.HelpEntries()
	return strings.Join(entries, "  ")
}

// KeyBindings returns key.Binding objects for integration with bubbles help system.
func (h *KeybindingHandler) KeyBindings() []key.Binding {
	keys := slices.Sorted(maps.Keys(h.keybindings))
	bindings := make([]key.Binding, 0, len(keys))

	for _, k := range keys {
		kb := h.keybindings[k]
		help := kb.Help

		// If keybinding doesn't override help, get from command
		if help == "" {
			if cmd, ok := h.commands[kb.Cmd]; ok {
				help = cmd.Help
				if help == "" && cmd.Action != "" {
					help = cmd.Action
				}
			}
		}
		if help == "" {
			help = "unknown"
		}

		bindings = append(bindings, key.NewBinding(
			key.WithKeys(k),
			key.WithHelp(k, help),
		))
	}

	return bindings
}

// ResolveUserCommand converts a user command to an Action ready for execution.
// The name is used to display the command source (e.g., ":review").
// Supports both action-based commands (recycle, delete) and shell commands.
func (h *KeybindingHandler) ResolveUserCommand(name string, cmd config.UserCommand, sess session.Session, args []string) Action {
	action := Action{
		Key:         ":" + name,
		Help:        cmd.Help,
		Confirm:     cmd.Confirm,
		SessionID:   sess.ID,
		SessionPath: sess.Path,
		Silent:      cmd.Silent,
		Exit:        cmd.ShouldExit(),
	}

	// Handle built-in actions
	if cmd.Action != "" {
		switch cmd.Action {
		case config.ActionRecycle:
			action.Type = ActionTypeRecycle
			if action.Help == "" {
				action.Help = "recycle"
			}
		case config.ActionDelete:
			action.Type = ActionTypeDelete
			if action.Help == "" {
				action.Help = "delete"
			}
		}
		return action
	}

	// Shell command
	data := struct {
		Path   string
		Remote string
		ID     string
		Name   string
		Args   []string
	}{
		Path:   sess.Path,
		Remote: sess.Remote,
		ID:     sess.ID,
		Name:   sess.Name,
		Args:   args,
	}

	rendered, err := tmpl.Render(cmd.Sh, data)
	if err != nil {
		// Surface template error instead of masking it
		action.Type = ActionTypeShell
		action.Err = fmt.Errorf("template error in command %q: %w", name, err)
		slog.Warn("template rendering failed",
			"command", name,
			"error", err)
		return action
	}

	action.Type = ActionTypeShell
	action.ShellCmd = rendered
	return action
}
