package review

import "github.com/colonyops/hive/internal/core/action"

// KeyResolver resolves keybindings to actions. Satisfied by the parent
// package's KeybindingResolver. Exists to avoid an import cycle.
type KeyResolver interface {
	// IsAction checks if a key maps to the given built-in action type.
	IsAction(key string, actionType action.Type) bool

	// ResolveAction resolves a key to an action without session context.
	ResolveAction(key string) (action.Action, bool)

	// HelpEntries returns formatted help strings for current view keybindings.
	HelpEntries() []string
}
