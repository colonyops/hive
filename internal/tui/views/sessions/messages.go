package sessions

import (
	tea "charm.land/bubbletea/v2"
	"github.com/colonyops/hive/internal/core/action"
	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/hive/plugins"
)

// --- Outbound messages (sessions view -> parent Model) ---

// ActionRequestMsg requests the parent to execute a resolved action.
type ActionRequestMsg struct {
	Action action.Action
}

// FormCommandRequestMsg requests the parent to show a form dialog for a command.
type FormCommandRequestMsg struct {
	Name    string
	Cmd     config.UserCommand
	Session session.Session
}

// CommandPaletteRequestMsg requests the parent to open the command palette.
type CommandPaletteRequestMsg struct {
	Session *session.Session
}

// NewSessionRequestMsg requests the parent to open the new session dialog.
type NewSessionRequestMsg struct{}

// RenameRequestMsg requests the parent to start a session rename flow.
type RenameRequestMsg struct {
	Session *session.Session
}

// DocReviewRequestMsg requests the parent to open the document review picker.
type DocReviewRequestMsg struct{}

// RecycledDeleteRequestMsg requests the parent to delete recycled sessions.
type RecycledDeleteRequestMsg struct {
	Sessions []session.Session
}

// OpenRepoRequestMsg requests the parent to open a repository (new session with preset remote).
type OpenRepoRequestMsg struct {
	Name   string
	Remote string
}

// RefreshSessionsMsg requests a session list refresh.
type RefreshSessionsMsg struct{}

// ErrorMsg signals a non-fatal error to the parent model.
type ErrorMsg struct{ Err error }

// ErrorCmd returns a tea.Cmd that emits ErrorMsg.
func ErrorCmd(err error) tea.Cmd {
	return func() tea.Msg { return ErrorMsg{Err: err} }
}

// --- Internal messages (stay within sessions package) ---
//
// GitStatusBatchCompleteMsg    -> gitstatus.go
// TerminalStatusBatchCompleteMsg -> terminalstatus.go
// TerminalPollTickMsg          -> terminalstatus.go

// sessionsLoadedMsg is sent when sessions are loaded from the store.
type sessionsLoadedMsg struct {
	sessions []session.Session
	err      error
}

// reposDiscoveredMsg is sent when repository scanning completes.
type reposDiscoveredMsg struct {
	repos []DiscoveredRepo
	err   error
}

// pluginWorkerStartedMsg is sent when the plugin background worker starts.
type pluginWorkerStartedMsg struct {
	resultsChan <-chan plugins.Result
}

// pluginStatusUpdateMsg is sent when a plugin status update is received.
type pluginStatusUpdateMsg struct {
	PluginName string
	SessionID  string
	Status     plugins.Status
	Err        error
}

// sessionRefreshTickMsg triggers a periodic session refresh.
type sessionRefreshTickMsg struct{}

// animationTickMsg triggers a status animation frame advance.
type animationTickMsg struct{}
