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
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/hive/plugins"
	"github.com/colonyops/hive/pkg/tmpl"
)

// Action is an alias for the unified action type.
type Action = action.Action

// DocTemplateData exposes the focused review-view document to user-command
// shell templates under the .Doc namespace.
type DocTemplateData struct {
	Path    string
	RelPath string
	Type    string
}

func docTemplateValue(doc *DocTemplateData) DocTemplateData {
	if doc == nil {
		return DocTemplateData{}
	}
	return *doc
}

// KeybindingResolver resolves keybindings to actions via UserCommands.
// It handles resolution only - execution is handled by the command.Service.
type KeybindingResolver struct {
	viewKeybindings        map[string]map[string]config.Keybinding // view name -> key -> binding
	effectiveKeybindings   map[string]config.Keybinding            // merged global + active view
	commandSet             *plugins.CommandSet
	renderer               *tmpl.Renderer
	activeView             ViewType                      // current active view for scope checking
	tmuxWindowLookup       func(sessionID string) string // optional: returns tmux target for a session
	toolLookup             func(sessionID string) string // optional: returns detected tool name for a session
	selectedWindowOverride string                        // if set, overrides tmuxWindowLookup for the next resolve
}

// NewKeybindingResolver creates a resolver. commandSet is the canonical
// command registry; the resolver reads from it on every IsAction/Resolve
// call, so mutations from any goroutine become visible immediately without
// rebuilding the resolver.
func NewKeybindingResolver(
	viewKeybindings map[string]map[string]config.Keybinding,
	commandSet *plugins.CommandSet,
	renderer *tmpl.Renderer,
) *KeybindingResolver {
	r := &KeybindingResolver{
		viewKeybindings: viewKeybindings,
		commandSet:      commandSet,
		renderer:        renderer,
		activeView:      ViewSessions, // default to sessions view
	}
	r.rebuildEffective()
	return r
}

// rebuildEffective merges global keybindings with the active view's keybindings.
// View-specific bindings override global bindings for the same key.
func (h *KeybindingResolver) rebuildEffective() {
	effective := make(map[string]config.Keybinding)
	if global, ok := h.viewKeybindings["global"]; ok {
		maps.Copy(effective, global)
	}
	viewName := h.activeView.String()
	if viewKBs, ok := h.viewKeybindings[viewName]; ok {
		maps.Copy(effective, viewKBs)
	}
	h.effectiveKeybindings = effective
}

// SetActiveView updates the current active view for scope checking
// and rebuilds the effective keybinding map.
func (h *KeybindingResolver) SetActiveView(view ViewType) {
	h.activeView = view
	h.rebuildEffective()
}

// SetTmuxWindowLookup sets a function that resolves tmux window or pane targets for sessions.
// This enables the legacy TmuxWindow field in shell command templates.
func (h *KeybindingResolver) SetTmuxWindowLookup(fn func(sessionID string) string) {
	h.tmuxWindowLookup = fn
}

// SetToolLookup sets a function that resolves tool names for sessions.
func (h *KeybindingResolver) SetToolLookup(fn func(sessionID string) string) {
	h.toolLookup = fn
}

// SetSelectedTarget overrides the legacy TmuxWindow template value for the next resolve call.
// The target may be a tmux window name/index or a pane ID such as %7.
// The override is consumed (cleared) after each Resolve or ResolveUserCommand call.
// Pass empty string to clear the override and fall back to the lookup function.
func (h *KeybindingResolver) SetSelectedTarget(target string) {
	h.selectedWindowOverride = target
}

// SetSelectedWindow overrides the TmuxWindow template value for the next resolve call.
//
// Deprecated: use SetSelectedTarget.
func (h *KeybindingResolver) SetSelectedWindow(windowName string) {
	h.SetSelectedTarget(windowName)
}

// consumeWindowOverride returns the selected tmux target for a session,
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
	cmd, exists := h.commandFor(key)
	return exists && cmd.Action == t
}

// IsCommand checks if a key maps to the given command name.
func (h *KeybindingResolver) IsCommand(key string, cmdName string) bool {
	kb, exists := h.effectiveKeybindings[key]
	if !exists {
		return false
	}
	return kb.Cmd == cmdName
}

// commandFor resolves key to a UserCommand after applying both key lookup and
// isCommandInScope filtering for the resolver's active view. Callers rely on
// this to distinguish missing bindings from commands that exist but are out of
// scope for the current dispatch path.
func (h *KeybindingResolver) commandFor(key string) (config.UserCommand, bool) {
	kb, exists := h.effectiveKeybindings[key]
	if !exists {
		return config.UserCommand{}, false
	}

	cmd, exists := h.commandSet.Lookup(kb.Cmd)
	if !exists || !h.isCommandInScope(cmd) {
		return config.UserCommand{}, false
	}

	return cmd, true
}

// renderUserCommandWindows renders a slice of WindowConfig templates against a UserCommand
// data map, returning pre-rendered WindowSpec values for use in a SpawnWindows action.
func renderUserCommandWindows(renderer *tmpl.Renderer, windows []config.WindowConfig, data map[string]any) ([]action.WindowSpec, error) {
	rws, err := hive.RenderUserCommandWindows(renderer, windows, data)
	if err != nil {
		return nil, err
	}
	specs := make([]action.WindowSpec, len(rws))
	for i, rw := range rws {
		specs[i] = action.WindowSpec{Name: rw.Name, Command: rw.Command, Dir: rw.Dir, Focus: rw.Focus}
		if len(rw.Panes) > 0 {
			specs[i].Panes = make([]action.PaneSpec, len(rw.Panes))
			for j, p := range rw.Panes {
				specs[i].Panes[j] = action.PaneSpec{Command: p.Command, Dir: p.Dir, Size: p.Size, Split: p.Split}
			}
		}
	}
	return specs, nil
}

// resolveWindowsAction builds a TypeSpawnWindows action from a UserCommand with windows.
// It routes sh: to the appropriate location depending on whether options.session_name is set.
func (h *KeybindingResolver) resolveWindowsAction(a Action, cmd config.UserCommand, sess session.Session, data map[string]any) Action {
	a.Type = action.TypeSpawnWindows

	windows, err := renderUserCommandWindows(h.renderer, cmd.Windows, data)
	if err != nil {
		a.Err = fmt.Errorf("template error in windows: %w", err)
		return a
	}

	// Build new-session request if options.session_name is set.
	var newSess *action.NewSessionRequest
	if cmd.Options.SessionName != "" {
		sessionName, err := h.renderer.Render(cmd.Options.SessionName, data)
		if err != nil {
			a.Err = fmt.Errorf("template error in options.session_name: %w", err)
			return a
		}
		remote := sess.Remote
		if cmd.Options.Remote != "" {
			rendered, err := h.renderer.Render(cmd.Options.Remote, data)
			if err != nil {
				a.Err = fmt.Errorf("template error in options.remote: %w", err)
				return a
			}
			remote = rendered
		}
		newSess = &action.NewSessionRequest{Name: sessionName, Remote: remote}
	}

	// Render sh: and route it to the right location.
	var shCmd, shDir string
	if cmd.Sh != "" {
		rendered, err := h.renderer.Render(cmd.Sh, data)
		if err != nil {
			a.Err = fmt.Errorf("template error in sh: %w", err)
			return a
		}
		if newSess != nil {
			// new-session mode: sh: runs after the git clone in the new session's path
			newSess.ShCmd = rendered
		} else {
			// same-session mode: sh: runs in the selected session's path
			shCmd = rendered
			shDir = sess.Path
		}
	}

	a.SpawnWindows = &action.SpawnWindowsPayload{
		ShCmd:      shCmd,
		ShDir:      shDir,
		Windows:    windows,
		TmuxTarget: sess.Name,
		SessionDir: sess.Path,
		Background: cmd.Options.Background,
		NewSession: newSess,
	}
	return a
}

// Resolve attempts to resolve a key press to an action for the given session.
// Recycled sessions only allow delete actions to prevent accidental operations.
func (h *KeybindingResolver) Resolve(key string, sess session.Session) (Action, bool) {
	kb, exists := h.effectiveKeybindings[key]
	if !exists {
		return Action{}, false
	}

	// Look up the referenced command
	cmd, cmdExists := h.commandSet.Lookup(kb.Cmd)
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
		a.Args = slices.Clone(cmd.Args)

		// Default help for actions that need it
		if a.Help == "" {
			a.Help = strings.ToLower(string(cmd.Action))
		}

		// Tmux actions need the selected target override.
		if cmd.Action == action.TypeTmuxOpen || cmd.Action == action.TypeTmuxStart {
			a.TmuxWindow = h.consumeWindowOverride(sess.ID)
		}

		return a, true
	}

	// Shell command or windows
	if cmd.Sh != "" || len(cmd.Windows) > 0 {
		data := map[string]any{
			"Path":       sess.Path,
			"Remote":     sess.Remote,
			"ID":         sess.ID,
			"Name":       sess.Name,
			"Tool":       h.toolForSession(sess.ID),
			"TmuxWindow": h.consumeWindowOverride(sess.ID),
		}

		if len(cmd.Windows) > 0 {
			return h.resolveWindowsAction(a, cmd, sess, data), true
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
		a.ShellDir = sess.Path
		return a, true
	}

	return Action{}, false
}

// ResolveAction resolves a key press to an action without session context.
// Used by views (like tasks) that don't operate on sessions.
// Only resolves built-in action commands -- shell commands are skipped.
func (h *KeybindingResolver) ResolveAction(key string) (Action, bool) {
	kb, exists := h.effectiveKeybindings[key]
	if !exists {
		return Action{}, false
	}

	cmd, cmdExists := h.commandSet.Lookup(kb.Cmd)
	if !cmdExists {
		log.Warn().Str("key", key).Str("cmd", kb.Cmd).Msg("keybinding references unknown command")
		return Action{}, false
	}

	if !h.isCommandInScope(cmd) {
		return Action{}, false
	}

	// Shell commands need session context - skip them
	if cmd.Action == "" {
		return Action{}, false
	}

	a := Action{
		Key:    key,
		Type:   cmd.Action,
		Args:   slices.Clone(cmd.Args),
		Help:   kb.Help,
		Silent: cmd.Silent,
	}
	if a.Help == "" {
		a.Help = cmd.Help
		if a.Help == "" {
			a.Help = strings.ToLower(string(cmd.Action))
		}
	}

	return a, true
}

// HelpEntries returns all configured keybindings for display, sorted by key.
// Only returns keybindings that are in scope for the current view.
func (h *KeybindingResolver) HelpEntries() []string {
	// Get sorted keys for consistent ordering
	keys := slices.Sorted(maps.Keys(h.effectiveKeybindings))

	entries := make([]string, 0, len(h.effectiveKeybindings))
	for _, key := range keys {
		kb := h.effectiveKeybindings[key]

		// Get command and check scope
		cmd, ok := h.commandSet.Lookup(kb.Cmd)
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
		entries = append(entries, fmt.Sprintf("%s %s", key, help))
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
	keys := slices.Sorted(maps.Keys(h.effectiveKeybindings))
	bindings := make([]key.Binding, 0, len(keys))

	for _, k := range keys {
		kb := h.effectiveKeybindings[k]

		// Get command and check scope
		cmd, ok := h.commandSet.Lookup(kb.Cmd)
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
// doc, when non-nil, populates the .Doc.* template namespace with the focused
// review-view document; otherwise .Doc fields render as empty strings.
func (h *KeybindingResolver) ResolveUserCommand(name string, cmd config.UserCommand, sess session.Session, args []string, doc *DocTemplateData) Action {
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
		a.Args = append(slices.Clone(cmd.Args), args...)

		if a.Help == "" {
			a.Help = strings.ToLower(string(cmd.Action))
		}

		if cmd.Action == action.TypeTmuxOpen || cmd.Action == action.TypeTmuxStart {
			a.TmuxWindow = h.consumeWindowOverride(sess.ID)
		}

		return a
	}

	// Shell command or windows
	data := map[string]any{
		"Path":       sess.Path,
		"Remote":     sess.Remote,
		"ID":         sess.ID,
		"Name":       sess.Name,
		"Tool":       h.toolForSession(sess.ID),
		"TmuxWindow": h.consumeWindowOverride(sess.ID),
		"Args":       args,
		"Doc":        docTemplateValue(doc),
	}

	if len(cmd.Windows) > 0 {
		return h.resolveWindowsAction(a, cmd, sess, data)
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
	a.ShellDir = sess.Path
	return a
}

// RenderWithFormData resolves a user command with form data injected
// into the template context under the .Form namespace.
// doc, when non-nil, populates the .Doc.* template namespace with the focused
// review-view document; otherwise .Doc fields render as empty strings.
func (h *KeybindingResolver) RenderWithFormData(
	name string,
	cmd config.UserCommand,
	sess session.Session,
	args []string,
	formData map[string]any,
	doc *DocTemplateData,
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
		"Doc":        docTemplateValue(doc),
	}

	if len(cmd.Windows) > 0 {
		return h.resolveWindowsAction(a, cmd, sess, data)
	}

	rendered, err := h.renderer.Render(cmd.Sh, data)
	if err != nil {
		a.Type = action.TypeShell
		a.Err = fmt.Errorf("template error in command %q: %w", name, err)
		return a
	}

	a.Type = action.TypeShell
	a.ShellCmd = rendered
	a.ShellDir = sess.Path
	return a
}

// ResolveFormCommand checks if a key maps to a user command with form fields.
// Returns the command name and command if found, after scope and recycle checks.
func (h *KeybindingResolver) ResolveFormCommand(key string, sess session.Session) (string, config.UserCommand, bool) {
	kb, exists := h.effectiveKeybindings[key]
	if !exists {
		return "", config.UserCommand{}, false
	}

	cmd, cmdExists := h.commandSet.Lookup(kb.Cmd)
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
