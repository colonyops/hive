package tui

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	"github.com/rs/zerolog/log"

	"github.com/colonyops/hive/internal/core/action"
	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/pkg/tmpl"
)

// Action is an alias for the unified action type.
type Action = action.Action

// KeybindingResolver resolves keybindings to actions via UserCommands.
// It handles resolution only - execution is handled by the command.Service.
type KeybindingResolver struct {
	keybindings            map[string]config.Keybinding
	commands               map[string]config.UserCommand
	renderer               *tmpl.Renderer
	activeView             ViewType                      // current active view for scope checking
	tmuxWindowLookup       func(sessionID string) string // optional: returns tmux window name for a session
	toolLookup             func(sessionID string) string // optional: returns detected tool name for a session
	selectedWindowOverride string                        // if set, overrides tmuxWindowLookup for the next resolve
}

// NewKeybindingResolver creates a new resolver with the given config.
// Commands should be the merged user commands (user config + system defaults).
func NewKeybindingResolver(keybindings map[string]config.Keybinding, commands map[string]config.UserCommand, renderer *tmpl.Renderer) *KeybindingResolver {
	return &KeybindingResolver{
		keybindings: keybindings,
		commands:    commands,
		renderer:    renderer,
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
func (h *KeybindingResolver) IsAction(key string, t action.Type) bool {
	kb, exists := h.keybindings[key]
	if !exists {
		return false
	}
	cmd, exists := h.commands[kb.Cmd]
	return exists && cmd.Action == t
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
	if sess.State == session.StateRecycled && cmd.Action != action.TypeDelete {
		return Action{}, false
	}

	// Build action from command, with keybinding overrides
	a := Action{
		Key:           key,
		Help:          kb.Help,
		Confirm:       kb.Confirm,
		SessionID:     sess.ID,
		SessionName:   sess.Name,
		SessionPath:   sess.Path,
		SessionRemote: sess.Remote,
		Silent:        cmd.Silent,
		Exit:          cmd.ShouldExit(),
	}

	// Use command values if keybinding doesn't override
	if a.Help == "" {
		a.Help = cmd.Help
	}
	if a.Confirm == "" {
		a.Confirm = cmd.Confirm
	}

	// Resolve action type from command
	if cmd.Action != "" {
		a.Type = cmd.Action

		// Default help for actions that need it
		if a.Help == "" {
			a.Help = strings.ToLower(string(cmd.Action))
		}

		// Tmux actions need window override
		if cmd.Action == action.TypeTmuxOpen || cmd.Action == action.TypeTmuxStart {
			a.TmuxWindow = h.consumeWindowOverride(sess.ID)
		}

		return a, true
	}

	// Shell command
	if cmd.Sh != "" {
		data := map[string]any{
			"Path":       sess.Path,
			"Remote":     sess.Remote,
			"ID":         sess.ID,
			"Name":       sess.Name,
			"Tool":       h.toolForSession(sess.ID),
			"TmuxWindow": h.consumeWindowOverride(sess.ID),
		}

		rendered, err := h.renderer.Render(cmd.Sh, data)
		if err != nil {
			// Surface template error instead of masking it
			a.Type = action.TypeShell
			a.Err = fmt.Errorf("template error in command %q: %w", kb.Cmd, err)
			log.Warn().Str("key", key).Str("cmd", kb.Cmd).Err(err).Msg("template rendering failed")
			return a, true
		}

		a.Type = action.TypeShell
		a.ShellCmd = rendered
		return a, true
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
				help = strings.ToLower(string(cmd.Action))
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
				help = strings.ToLower(string(cmd.Action))
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
	a := Action{
		Key:           ":" + name,
		Help:          cmd.Help,
		Confirm:       cmd.Confirm,
		SessionID:     sess.ID,
		SessionName:   sess.Name,
		SessionPath:   sess.Path,
		SessionRemote: sess.Remote,
		Silent:        cmd.Silent,
		Exit:          cmd.ShouldExit(),
	}

	// Handle built-in actions
	if cmd.Action != "" {
		a.Type = cmd.Action

		if a.Help == "" {
			a.Help = strings.ToLower(string(cmd.Action))
		}

		if cmd.Action == action.TypeTmuxOpen || cmd.Action == action.TypeTmuxStart {
			a.TmuxWindow = h.consumeWindowOverride(sess.ID)
		}

		return a
	}

	// Shell command
	data := map[string]any{
		"Path":       sess.Path,
		"Remote":     sess.Remote,
		"ID":         sess.ID,
		"Name":       sess.Name,
		"Tool":       h.toolForSession(sess.ID),
		"TmuxWindow": h.consumeWindowOverride(sess.ID),
		"Args":       args,
	}

	rendered, err := h.renderer.Render(cmd.Sh, data)
	if err != nil {
		a.Type = action.TypeShell
		a.Err = fmt.Errorf("template error in command %q: %w", name, err)
		log.Warn().Str("command", name).Err(err).Msg("template rendering failed")
		return a
	}

	a.Type = action.TypeShell
	a.ShellCmd = rendered
	return a
}

// RenderWithFormData resolves a user command with form data injected
// into the template context under the .Form namespace.
func (h *KeybindingResolver) RenderWithFormData(
	name string,
	cmd config.UserCommand,
	sess session.Session,
	args []string,
	formData map[string]any,
) Action {
	a := Action{
		Key:           ":" + name,
		Help:          cmd.Help,
		Confirm:       cmd.Confirm,
		SessionID:     sess.ID,
		SessionName:   sess.Name,
		SessionPath:   sess.Path,
		SessionRemote: sess.Remote,
		Silent:        cmd.Silent,
		Exit:          cmd.ShouldExit(),
	}

	data := map[string]any{
		"Path":       sess.Path,
		"Remote":     sess.Remote,
		"ID":         sess.ID,
		"Name":       sess.Name,
		"Tool":       h.toolForSession(sess.ID),
		"TmuxWindow": h.consumeWindowOverride(sess.ID),
		"Args":       args,
		"Form":       formData,
	}

	rendered, err := h.renderer.Render(cmd.Sh, data)
	if err != nil {
		a.Type = action.TypeShell
		a.Err = fmt.Errorf("template error in command %q: %w", name, err)
		return a
	}

	a.Type = action.TypeShell
	a.ShellCmd = rendered
	return a
}

// ResolveFormCommand checks if a key maps to a user command with form fields.
// Returns the command name and command if found, after scope and recycle checks.
func (h *KeybindingResolver) ResolveFormCommand(key string, sess session.Session) (string, config.UserCommand, bool) {
	kb, exists := h.keybindings[key]
	if !exists {
		return "", config.UserCommand{}, false
	}

	cmd, cmdExists := h.commands[kb.Cmd]
	if !cmdExists || len(cmd.Form) == 0 {
		return "", config.UserCommand{}, false
	}

	if !h.isCommandInScope(cmd) {
		return "", config.UserCommand{}, false
	}

	if sess.State == session.StateRecycled {
		return "", config.UserCommand{}, false
	}

	return kb.Cmd, cmd, true
}
