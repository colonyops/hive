package action

// WindowSpec is a fully-rendered window definition carried in a SpawnWindows action.
// Fields mirror coretmux.RenderedWindow to avoid a cross-package dependency on the tmux layer.
type WindowSpec struct {
	Name    string
	Command string
	Dir     string
	Focus   bool
}

// NewSessionRequest carries parameters for creating a new Hive session before spawning windows.
type NewSessionRequest struct {
	Name   string // Rendered session name
	Remote string // Remote URL; empty = inherit from selected session
	// ShCmd is an optional shell command run in the new session's directory after the git clone
	// and before windows are opened. Non-zero exit aborts window creation.
	ShCmd string
}

// SpawnWindowsPayload is the execution payload for TypeSpawnWindows actions.
type SpawnWindowsPayload struct {
	// Optional sh: command to run before opening windows in same-session mode.
	// For new-session mode, ShCmd lives on NewSession.ShCmd instead.
	ShCmd string
	ShDir string // Working directory for ShCmd (same-session mode only)

	// Windows to open.
	Windows []WindowSpec

	// Target for same-session mode (TmuxTarget = existing session's tmux name).
	TmuxTarget string
	SessionDir string // Working directory fallback for window dir resolution
	Background bool

	// New-session mode: if non-nil, a Hive session is created before windows are opened.
	// ShCmd is NOT used in this mode; use NewSession.ShCmd instead.
	NewSession *NewSessionRequest
}

// Action represents a resolved keybinding or command action ready for execution.
type Action struct {
	Type          Type
	Key           string
	Help          string
	Confirm       string               // Non-empty if confirmation required
	ShellCmd      string               // For shell actions, the rendered command
	ShellDir      string               // Working directory for TypeShell (empty = hive process cwd)
	SpawnWindows  *SpawnWindowsPayload // For TypeSpawnWindows
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
	TypeNotifications:  true,
	TypeRenameSession:  true,
	TypeNextActive:     true,
	TypePrevActive:     true,
	TypeHiveInfo:       true,
	TypeHiveDoctor:     true,
	TypeSetGroup:       true,
}

// IsConfigAction reports whether t is a valid action for use in YAML config.
func IsConfigAction(t Type) bool {
	return configActions[t]
}
