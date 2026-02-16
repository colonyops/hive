package action

// Action represents a resolved keybinding or command action ready for execution.
type Action struct {
	Type          Type
	Key           string
	Help          string
	Confirm       string // Non-empty if confirmation required
	ShellCmd      string // For shell actions, the rendered command
	SessionID     string
	SessionName   string // Session display name (for tmux actions)
	SessionPath   string
	SessionRemote string // Session remote URL (for tmux actions)
	TmuxWindow    string // Target tmux window name (for tmux actions)
	Silent        bool   // Skip loading popup for fast commands
	Exit          bool   // Exit hive after command completes
	Err           error  // Non-nil if action resolution failed (e.g., template error)
}

// NeedsConfirm returns true if the action requires user confirmation.
func (a Action) NeedsConfirm() bool {
	return a.Confirm != ""
}

// configActions are action types that can be set via the YAML config action field.
// Shell, None, and DeleteRecycledBatch are internal-only.
var configActions = map[Type]bool{
	TypeRecycle:        true,
	TypeDelete:         true,
	TypeTmuxOpen:       true,
	TypeTmuxStart:      true,
	TypeFilterAll:      true,
	TypeFilterActive:   true,
	TypeFilterApproval: true,
	TypeFilterReady:    true,
	TypeDocReview:      true,
	TypeNewSession:     true,
	TypeSetTheme:       true,
	TypeMessages:       true,
	TypeRenameSession:  true,
	TypeNextActive:     true,
	TypePrevActive:     true,
}

// IsConfigAction reports whether t is a valid action for use in YAML config.
func IsConfigAction(t Type) bool {
	return configActions[t]
}
