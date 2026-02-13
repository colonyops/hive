package tui

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	"github.com/rs/zerolog/log"

	"github.com/hay-kot/hive/internal/core/config"
	"github.com/hay-kot/hive/internal/core/session"
	"github.com/hay-kot/hive/pkg/tmpl"
)

// ActionType identifies the kind of action a keybinding triggers.
type ActionType int

const (
	ActionTypeNone ActionType = iota
	ActionTypeRecycle
	ActionTypeDelete
	ActionTypeShell
	ActionTypeFilterAll
	ActionTypeFilterActive
	ActionTypeFilterApproval
	ActionTypeFilterReady
	ActionTypeDocReview
	ActionTypeNewSession
	ActionTypeSetTheme
	ActionTypeMessages
	ActionTypeRenameSession
	ActionTypeNextActive
	ActionTypePrevActive
	ActionTypeDeleteRecycledBatch // Delete all recycled sessions at once (must stay at end to not shift command.ActionType values)
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

// KeybindingResolver resolves keybindings to actions via UserCommands.
// It handles resolution only - execution is handled by the command.Service.
type KeybindingResolver struct {
	keybindings            map[string]config.Keybinding
	commands               map[string]config.UserCommand
	activeView             ViewType                      // current active view for scope checking
	tmuxWindowLookup       func(sessionID string) string // optional: returns tmux window name for a session
	toolLookup             func(sessionID string) string // optional: returns detected tool name for a session
	selectedWindowOverride string                        // if set, overrides tmuxWindowLookup for the next resolve
}

// NewKeybindingResolver creates a new resolver with the given config.
// Commands should be the merged user commands (user config + system defaults).
func NewKeybindingResolver(keybindings map[string]config.Keybinding, commands map[string]config.UserCommand) *KeybindingResolver {
	return &KeybindingResolver{
		keybindings: keybindings,
		commands:    commands,
		activeView:  ViewSessions, // default to sessions view
	}
}

// SetActiveView updates the current active view for scope checking.
func (h *KeybindingResolver) SetActiveView(view ViewType) {
	h.activeView = view
}

// SetTmuxWindowLookup sets a function that resolves tmux window names for sessions.
// This enables the TmuxWindow field in shell command templates.
func (h *KeybindingResolver) SetTmuxWindowLookup(fn func(sessionID string) string) {
	h.tmuxWindowLookup = fn
}

// SetToolLookup sets a function that resolves tool names for sessions.
func (h *KeybindingResolver) SetToolLookup(fn func(sessionID string) string) {
	h.toolLookup = fn
}

// SetSelectedWindow overrides the TmuxWindow template value for the next resolve call.
// The override is consumed (cleared) after each Resolve or ResolveUserCommand call.
// Pass empty string to clear the override and fall back to the lookup function.
func (h *KeybindingResolver) SetSelectedWindow(windowName string) {
	h.selectedWindowOverride = windowName
}

// consumeWindowOverride returns the current selectedWindowOverride for a session,
// falling back to the lookup function, and clears the override afterward.
func (h *KeybindingResolver) consumeWindowOverride(sessionID string) string {
	if h.selectedWindowOverride != "" {
		result := h.selectedWindowOverride
		h.selectedWindowOverride = ""
		return result
	}
	if h.tmuxWindowLookup == nil {
		return ""
	}
	return h.tmuxWindowLookup(sessionID)
}

// toolForSession returns the detected tool name for a session, or empty.
func (h *KeybindingResolver) toolForSession(sessionID string) string {
	if h.toolLookup == nil {
		return ""
	}
	return h.toolLookup(sessionID)
}

// isCommandInScope checks if a command is active in the current view.
func (h *KeybindingResolver) isCommandInScope(cmd config.UserCommand) bool {
	if len(cmd.Scope) == 0 {
		return true // global by default
	}
	currentScope := h.activeView.String()
	for _, scope := range cmd.Scope {
		if scope == "global" || scope == currentScope {
			return true
		}
	}
	return false
}

// IsAction checks if a key maps to the given built-in action.
func (h *KeybindingResolver) IsAction(key string, action string) bool {
	kb, exists := h.keybindings[key]
	if !exists {
		return false
	}
	cmd, exists := h.commands[kb.Cmd]
	return exists && cmd.Action == action
}

// IsCommand checks if a key maps to the given command name.
func (h *KeybindingResolver) IsCommand(key string, cmdName string) bool {
	kb, exists := h.keybindings[key]
	if !exists {
		return false
	}
	return kb.Cmd == cmdName
}

// Resolve attempts to resolve a key press to an action for the given session.
// Recycled sessions only allow delete actions to prevent accidental operations.
func (h *KeybindingResolver) Resolve(key string, sess session.Session) (Action, bool) {
	kb, exists := h.keybindings[key]
	if !exists {
		return Action{}, false
	}

	// Look up the referenced command
	cmd, cmdExists := h.commands[kb.Cmd]
	if !cmdExists {
		// Command reference is invalid - validation should catch this,
		// but log and return gracefully for debugging
		log.Warn().Str("key", key).Str("cmd", kb.Cmd).Msg("keybinding references unknown command")
		return Action{}, false
	}

	// Check if command is in scope for current view
	if !h.isCommandInScope(cmd) {
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
		case config.ActionFilterAll:
			action.Type = ActionTypeFilterAll
		case config.ActionFilterActive:
			action.Type = ActionTypeFilterActive
		case config.ActionFilterApproval:
			action.Type = ActionTypeFilterApproval
		case config.ActionFilterReady:
			action.Type = ActionTypeFilterReady
		case config.ActionDocReview:
			action.Type = ActionTypeDocReview
			if action.Help == "" {
				action.Help = "review document"
			}
		case config.ActionSetTheme:
			action.Type = ActionTypeSetTheme
		case config.ActionMessages:
			action.Type = ActionTypeMessages
		case config.ActionRenameSession:
			action.Type = ActionTypeRenameSession
			if action.Help == "" {
				action.Help = "rename session"
			}
		case config.ActionNextActive:
			action.Type = ActionTypeNextActive
		case config.ActionPrevActive:
			action.Type = ActionTypePrevActive
		}
		return action, true
	}

	// Shell command
	if cmd.Sh != "" {
		data := struct {
			Path       string
			Remote     string
			ID         string
			Name       string
			Tool       string
			TmuxWindow string
		}{
			Path:       sess.Path,
			Remote:     sess.Remote,
			ID:         sess.ID,
			Name:       sess.Name,
			Tool:       h.toolForSession(sess.ID),
			TmuxWindow: h.consumeWindowOverride(sess.ID),
		}

		rendered, err := tmpl.Render(cmd.Sh, data)
		if err != nil {
			// Surface template error instead of masking it
			action.Type = ActionTypeShell
			action.Err = fmt.Errorf("template error in command %q: %w", kb.Cmd, err)
			log.Warn().Str("key", key).Str("cmd", kb.Cmd).Err(err).Msg("template rendering failed")
			return action, true
		}

		action.Type = ActionTypeShell
		action.ShellCmd = rendered
		return action, true
	}

	return Action{}, false
}

// HelpEntries returns all configured keybindings for display, sorted by key.
// Only returns keybindings that are in scope for the current view.
func (h *KeybindingResolver) HelpEntries() []string {
	// Get sorted keys for consistent ordering
	keys := slices.Sorted(maps.Keys(h.keybindings))

	entries := make([]string, 0, len(h.keybindings))
	for _, key := range keys {
		kb := h.keybindings[key]

		// Get command and check scope
		cmd, ok := h.commands[kb.Cmd]
		if !ok || !h.isCommandInScope(cmd) {
			continue // skip out-of-scope commands
		}

		help := kb.Help

		// If keybinding doesn't override help, get from command
		if help == "" {
			help = cmd.Help
			if help == "" && cmd.Action != "" {
				help = cmd.Action
			}
		}
		if help == "" {
			help = unknownViewType
		}
		entries = append(entries, fmt.Sprintf("[%s] %s", key, help))
	}
	return entries
}

// HelpString returns a formatted help string for all keybindings.
func (h *KeybindingResolver) HelpString() string {
	entries := h.HelpEntries()
	return strings.Join(entries, "  ")
}

// KeyBindings returns key.Binding objects for integration with bubbles help system.
// Only returns keybindings that are in scope for the current view.
func (h *KeybindingResolver) KeyBindings() []key.Binding {
	keys := slices.Sorted(maps.Keys(h.keybindings))
	bindings := make([]key.Binding, 0, len(keys))

	for _, k := range keys {
		kb := h.keybindings[k]

		// Get command and check scope
		cmd, ok := h.commands[kb.Cmd]
		if !ok || !h.isCommandInScope(cmd) {
			continue // skip out-of-scope commands
		}

		help := kb.Help

		// If keybinding doesn't override help, get from command
		if help == "" {
			help = cmd.Help
			if help == "" && cmd.Action != "" {
				help = cmd.Action
			}
		}
		if help == "" {
			help = unknownViewType
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
func (h *KeybindingResolver) ResolveUserCommand(name string, cmd config.UserCommand, sess session.Session, args []string) Action {
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
		case config.ActionFilterAll:
			action.Type = ActionTypeFilterAll
		case config.ActionFilterActive:
			action.Type = ActionTypeFilterActive
		case config.ActionFilterApproval:
			action.Type = ActionTypeFilterApproval
		case config.ActionFilterReady:
			action.Type = ActionTypeFilterReady
		case config.ActionSetTheme:
			action.Type = ActionTypeSetTheme
		case config.ActionMessages:
			action.Type = ActionTypeMessages
		case config.ActionRenameSession:
			action.Type = ActionTypeRenameSession
			if action.Help == "" {
				action.Help = "rename session"
			}
		case config.ActionNextActive:
			action.Type = ActionTypeNextActive
		case config.ActionPrevActive:
			action.Type = ActionTypePrevActive
		}
		return action
	}

	// Shell command
	data := struct {
		Path       string
		Remote     string
		ID         string
		Name       string
		Tool       string
		TmuxWindow string
		Args       []string
	}{
		Path:       sess.Path,
		Remote:     sess.Remote,
		ID:         sess.ID,
		Name:       sess.Name,
		Tool:       h.toolForSession(sess.ID),
		TmuxWindow: h.consumeWindowOverride(sess.ID),
		Args:       args,
	}

	rendered, err := tmpl.Render(cmd.Sh, data)
	if err != nil {
		// Surface template error instead of masking it
		action.Type = ActionTypeShell
		action.Err = fmt.Errorf("template error in command %q: %w", name, err)
		log.Warn().Str("command", name).Err(err).Msg("template rendering failed")
		return action
	}

	action.Type = ActionTypeShell
	action.ShellCmd = rendered
	return action
}
