package events

// The complete core event vocabulary. A payload carries what happened, not a
// hint that something did: an adapter that only needs a wake-up can throw the
// payload away, but one that needs the delta cannot invent it.
//
// window:focus and window:blur are deliberately absent. Window focus is
// GUI-only state and stays entirely inside the wailsui adapter.

// LogAppended reports that a producer tick or webhook delivery appended at
// least one row to the event log.
type LogAppended struct{ NextOffset int64 }

// InboxUpdated reports that the flow engine committed at least one run — feed
// membership, queued actions, or node-run metrics may all have changed.
//
// This is a distinct event from LogAppended, not a duplicate of it. The log
// growing says a source observed something; it says nothing about whether any
// flow routed it anywhere. A reader that refreshes on LogAppended reads before
// the engine has committed, and one that never sees this event misses a
// membership change a replay made with no new log rows at all.
type InboxUpdated struct{}

// ActivityAppended reports a new row in the user-facing activity log.
type ActivityAppended struct{ ID int64 }

// JobsUpdated carries no status because publishers only know the job ID.
type JobsUpdated struct {
	JobID int64
}

// SessionCreateFailed reports that a New Session form was accepted and the
// session it asked for was not created. The failure is state the reader
// re-reads, so the payload names the attempt rather than carrying the reason.
type SessionCreateFailed struct{ Name string }

// SessionsUpdated reports the hive session ids that appeared, changed, or
// went away since the last read, whichever process wrote them.
type SessionsUpdated struct{ Added, Changed, Removed []string }

// TasksUpdated reports that an hc item, comment, or blocker was written since
// the last read, whichever process wrote it. The read is a fingerprint, so
// there are no ids to carry.
type TasksUpdated struct{}

// FlowsUpdated reports that the flow set was reloaded. Reason names what
// caused it — an external edit, or the app's own save.
type FlowsUpdated struct{ Reason string }

// ActionsUpdated reports that the actions catalog was reloaded or mutated.
type ActionsUpdated struct{ Count int }

type MenuBarUpdated struct{}

// AgentWorkspacesUpdated reports that the workspace set was reloaded.
type AgentWorkspacesUpdated struct{ Count int }

// DriveChanged reports that documents in a drive space changed: Paths names
// them, and empty means the whole space. The content is stored state a
// reader re-reads. Session or HiveSession names the chat or hive session
// whose agent wrote through the desktop, never both; both are empty for a
// change the drive watcher found on disk, which wakes a Drive tab without
// marking anything unseen.
type DriveChanged struct {
	Space       string
	Paths       []string
	Session     string
	HiveSession string
}

// SessionLayoutRequested reports an agent asked to change the panel layout
// beside its chat or Code session. Unlike the wake-up events the payload is
// the whole message: the layout is UI intent held by the browser, not stored
// state to re-read. The asker is a chat (Session) or a hive session
// (HiveSession), never both.
//
// Verb is open, close, move, focus, close_panel, resize or turn. View and
// Window name the tab acted on: View is drive, diff, tasks or terminal, and
// Window is the tmux window id (@3) of a Code session's terminal, empty for
// the other views and for a chat's one terminal. AnchorView and AnchorWindow
// name the tab an open or move places relative to, empty for the tab in front
// of the focused panel. Path pins one document of Space in the Drive view on
// open; empty leaves the tab's own pick. Placement is behind, beside or below
// and applies to open and move. Share is the resize share, 0 when unused.
type SessionLayoutRequested struct {
	Session      string
	HiveSession  string
	Space        string
	Verb         string
	View         string
	Window       string
	AnchorView   string
	AnchorWindow string
	Path         string
	Placement    string
	Share        float64
}

// SchedulesUpdated reports that a workspace's manifest was written or one of
// its schedules ran. The payload names only the workspace: the entry and its
// run history are both stored state a reader re-reads.
type SchedulesUpdated struct{ Workspace string }

// ConnectionUpdated reports that one provider's stored credentials changed —
// connected, rotated, or disconnected. Provider names which ("github"), so a
// consumer can ignore a provider it does not use; the new state is not in the
// payload because the connector's own status is the authority on it.
type ConnectionUpdated struct{ Provider string }

// NotificationRaised reports that a flow's notify terminal delivered a
// notification. Only a successful delivery is reported -- dispatch retries a
// failed one rather than treating it as having fired, so publishing early
// would announce something the user never saw. InApp carries the delivery
// policy's routing decision: wailsui uses it to decide whether to also raise
// an in-app toast, since an OS banner already went out through the notifier
// port itself by the time this publishes.
type NotificationRaised struct {
	ProfileID string
	ItemID    int64
	Title     string
	Body      string
	Severity  string
	InApp     bool
}

func (LogAppended) eventName() string            { return "log.appended" }
func (InboxUpdated) eventName() string           { return "inbox.updated" }
func (ActivityAppended) eventName() string       { return "activity.appended" }
func (JobsUpdated) eventName() string            { return "jobs.updated" }
func (SessionCreateFailed) eventName() string    { return "session.create-failed" }
func (SessionsUpdated) eventName() string        { return "sessions.updated" }
func (TasksUpdated) eventName() string           { return "tasks.updated" }
func (FlowsUpdated) eventName() string           { return "flows.updated" }
func (ActionsUpdated) eventName() string         { return "actions.updated" }
func (MenuBarUpdated) eventName() string         { return "menu-bar.updated" }
func (AgentWorkspacesUpdated) eventName() string { return "agent-workspaces.updated" }
func (DriveChanged) eventName() string           { return "drive.changed" }
func (SessionLayoutRequested) eventName() string { return "session.layout-requested" }
func (SchedulesUpdated) eventName() string       { return "schedules.updated" }
func (ConnectionUpdated) eventName() string      { return "connection.updated" }
func (NotificationRaised) eventName() string     { return "notification.raised" }
