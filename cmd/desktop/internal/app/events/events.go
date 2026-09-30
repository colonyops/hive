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

// FlowsUpdated reports that the flow set was reloaded. Reason names what
// caused it — an external edit, or the app's own save.
type FlowsUpdated struct{ Reason string }

// ActionsUpdated reports that the actions catalog was reloaded or mutated.
type ActionsUpdated struct{ Count int }

type MenuBarUpdated struct{}

// AgentWorkspacesUpdated reports that the workspace set was reloaded.
type AgentWorkspacesUpdated struct{ Count int }

// CanvasUpdated reports that a canvas in the authoring session's workspace
// changed — a block was put or removed, a canvas deleted or cleared. The
// content is stored state a reader re-reads; the payload names no canvas,
// only the session, and the pane re-reads its workspace's canvases.
type CanvasUpdated struct{ Session int64 }

// CanvasToggleRequested reports an agent asked to open or close the canvas
// pane beside its chat. Unlike the wake-up events the payload is the whole
// message — pane visibility is UI intent, not stored state to re-read. Name
// pins one canvas when opening; empty leaves the pane's own pick.
type CanvasToggleRequested struct {
	Session int64
	Name    string
	Open    bool
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
func (FlowsUpdated) eventName() string           { return "flows.updated" }
func (ActionsUpdated) eventName() string         { return "actions.updated" }
func (MenuBarUpdated) eventName() string         { return "menu-bar.updated" }
func (AgentWorkspacesUpdated) eventName() string { return "agent-workspaces.updated" }
func (CanvasUpdated) eventName() string          { return "canvas.updated" }
func (CanvasToggleRequested) eventName() string  { return "canvas.toggle-requested" }
func (SchedulesUpdated) eventName() string       { return "schedules.updated" }
func (ConnectionUpdated) eventName() string      { return "connection.updated" }
func (NotificationRaised) eventName() string     { return "notification.raised" }
