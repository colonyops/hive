package sessions

import (
	"github.com/colonyops/hive/cmd/hive/internal/action"
	"github.com/colonyops/hive/cmd/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/session"
)

// KeyResolver resolves keybindings to actions. It is satisfied by the parent
// package's KeybindingResolver and exists to avoid an import cycle between the
// sessions view and the tui package.
type KeyResolver interface {
	// IsAction checks if a key maps to the given built-in action type.
	IsAction(key string, actionType action.Type) bool

	// IsCommand checks if a key maps to the given command name.
	IsCommand(key string, cmdName string) bool

	// Resolve attempts to resolve a key press to an action for the given session.
	Resolve(key string, sess session.Session) (action.Action, bool)

	// ResolveFormCommand checks if a key maps to a user command with form fields.
	ResolveFormCommand(key string, sess session.Session) (string, config.UserCommand, bool)

	// SetSelectedTarget overrides the TmuxWindow template value with a window target or pane ID.
	SetSelectedTarget(target string)

	// HelpEntries returns formatted help strings for current view keybindings.
	HelpEntries() []string
}
