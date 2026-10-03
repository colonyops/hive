package action

import "github.com/colonyops/hive/internal/core/multiplexer"

// SpawnWindowsPayload is the execution payload for TypeSpawnWindows actions.
type SpawnWindowsPayload struct {
	// ShCmd is an optional sh: command run before opening windows: in ShDir in
	// same-session mode, in the new session's clone in new-session mode.
	ShCmd string
	ShDir string

	Windows []multiplexer.WindowSpec

	// Target for same-session mode (TmuxTarget = existing session's tmux name).
	TmuxTarget string
	SessionDir string // Working directory fallback for window dir resolution
	Background bool

	// NewSession selects new-session mode: a Hive session named NewSessionName
	// is cloned from NewSessionRemote before windows are opened.
	NewSession       bool
	NewSessionName   string
	NewSessionRemote string
}

// Action represents a resolved keybinding or command action ready for execution.
type Action struct {
	Type Type
	// Args carries the command's preset args (config.UserCommand.Args) for
	// built-in actions that accept arguments (e.g. OpenSourcePicker's
	// source id/scope).
	Args          []string
	Key           string
	Help          string
	Confirm       string               // Non-empty if confirmation required
	ShellCmd      string               // For shell actions, the rendered command
	ShellDir      string               // Working directory for TypeShell (empty = hive process cwd)
	SpawnWindows  *SpawnWindowsPayload // For TypeSpawnWindows
	WindowTarget  *multiplexer.Target  // For TypeKillWindow
	SessionID     string
	SessionName   string // Session display name (for tmux actions)
	SessionPath   string
	SessionRemote string // Session remote URL (for tmux actions)
	// TmuxWindow carries the resolved tmux target (window name/index or pane ID)
	// for TmuxOpen and TmuxStart actions. The name predates pane-level targeting;
	// SpawnWindowsPayload.TmuxTarget is preferred for new spawn actions.
	TmuxWindow string
	Silent     bool  // Skip loading popup for fast commands
	Exit       bool  // Exit hive after command completes
	Err        error // Non-nil if action resolution failed (e.g., template error)
}

// NeedsConfirm returns true if the action requires user confirmation.
func (a Action) NeedsConfirm() bool {
	return a.Confirm != ""
}

// configActions are action types that can be set via the YAML config action field.
// Shell, None, and DeleteRecycledBatch are internal-only.
var configActions = map[Type]bool{
	TypeRecycle:          true,
	TypeDelete:           true,
	TypeTmuxOpen:         true,
	TypeTmuxStart:        true,
	TypeFilterAll:        true,
	TypeFilterActive:     true,
	TypeFilterApproval:   true,
	TypeFilterReady:      true,
	TypeDocReview:        true,
	TypeNewSession:       true,
	TypeSetTheme:         true,
	TypeNotifications:    true,
	TypeRenameSession:    true,
	TypeNextActive:       true,
	TypePrevActive:       true,
	TypeHiveInfo:         true,
	TypeHiveDoctor:       true,
	TypeGroupSet:         true,
	TypeGroupToggle:      true,
	TypeTodoPanel:        true,
	TypeOpenSourcePicker: true,

	TypeTasksRefresh:       true,
	TypeTasksFilter:        true,
	TypeTasksCopyID:        true,
	TypeTasksTogglePreview: true,
	TypeTasksSetOpen:       true,
	TypeTasksSetInProgress: true,
	TypeTasksSetDone:       true,
	TypeTasksSetCancelled:  true,
	TypeTasksDelete:        true,
	TypeTasksPrune:         true,
}

// IsConfigAction reports whether t is a valid action for use in YAML config.
func IsConfigAction(t Type) bool {
	return configActions[t]
}
