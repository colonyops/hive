package wailsui

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/colonyops/hive/cmd/desktop/internal/app/events"

	"github.com/wailsapp/wails/v3/pkg/application"
	wailsnotify "github.com/wailsapp/wails/v3/pkg/services/notifications"
)

// Every event this adapter emits is registered and emitted from this file.
// They are wake-up signals: the frontend re-reads the relevant service on
// receipt, and only log:appended, notification:activated, notification:toast,
// menubar:open and update:available carry a payload that matters.

// Package-variable initialization instead of init(): this repo enables
// gochecknoinits.
var _ = registerEvents()

func registerEvents() struct{} {
	application.RegisterEvent[TerminalFilesDropped]("terminal:files-dropped")
	// connection:updated carries the provider whose credentials changed;
	// log:appended carries the pipeline event log's new tail offset after a
	// producer tick appends at least one row; flows:updated fires after a
	// flows/*.yaml directory reload (an external edit, or the app's own
	// SaveFlow/SaveLayout — see buildFlowsStore); actions:updated fires after
	// an actions.yml reload. All are wake-up signals: the frontend re-reads
	// the relevant service on receipt.
	application.RegisterEvent[string]("connection:updated")
	application.RegisterEvent[int64]("log:appended")
	// inbox:updated fires after the flow engine commits at least one run. It
	// is the signal a feed re-read keys off: log:appended only says a source
	// observed something, which may route nowhere at all.
	application.RegisterEvent[string]("inbox:updated")
	application.RegisterEvent[string]("flows:updated")
	application.RegisterEvent[string]("actions:updated")
	application.RegisterEvent[string]("jobs:updated")
	// sessions:create-failed names the session a create job failed to produce.
	// A wake-up like the rest: the frontend re-reads the failed draft to
	// restore the form. It exists because a create that fails minutes after
	// the dialog closed has to arrive at the user, not wait in a list.
	application.RegisterEvent[string]("sessions:create-failed")
	application.RegisterEvent[string]("sessions:updated")
	application.RegisterEvent[string]("tasks:updated")
	// window:focus and window:blur carry the current focus state. Consumers use
	// them to update focus-sensitive UI without querying the native window.
	application.RegisterEvent[bool]("window:focus")
	application.RegisterEvent[bool]("window:blur")
	// activity:appended carries the new event's id after any subsystem (or the
	// frontend, via ActivityService.Record) appends to the activity log. The
	// Activity view re-reads its latest page and advances its unseen marker.
	application.RegisterEvent[int64]("activity:appended")
	// canvas:updated carries the chat or hive session whose canvas an agent
	// just wrote. Canvas content is stored state, so the pane re-reads the
	// canvas it is showing on receipt; coalescing can drop an author but never
	// content.
	application.RegisterEvent[CanvasWrite]("canvas:updated")
	// canvas:toggle carries an agent's ask to open or close the canvas pane
	// beside its chat. Pane visibility is UI intent, not state to re-read, so
	// the payload is the whole message; coalescing keeps only the latest ask,
	// which is the final intent anyway.
	application.RegisterEvent[CanvasToggle]("canvas:toggle")
	// schedules:updated carries the workspace whose scheduled chats changed:
	// its manifest was written, or one of its schedules ran. The Chats area
	// re-reads its workspace and chat lists on receipt.
	application.RegisterEvent[string]("schedules:updated")
	// update:available carries the latest UpdateInfo when a self-update check
	// finds a newer desktop release; the title bar reacts to it.
	application.RegisterEvent[UpdateInfo]("update:available")
	// notification:activated carries the workspace and inbox item behind a
	// native notification the user clicked. Unlike the wake-up signals above
	// its payload is the whole message: the window is already being raised by
	// the time it fires, and the frontend's job is only to route to that item.
	application.RegisterEvent[NotificationActivation]("notification:activated")
	// notification:toast carries a flow notification the user chose to receive
	// inside Hive rather than as an OS banner (Settings -> Notifications ->
	// Delivery). The frontend surfaces it through the same toast stack every
	// other in-app notification uses.
	application.RegisterEvent[NotificationToast]("notification:toast")
	// The tray raises the window before emitting menubar:open.
	application.RegisterEvent[MenuBarNavigation]("menubar:open")
	return struct{}{}
}

// Subscribe wires every core event to its Wails wake-up signal and returns a
// cancel that tears every subscription down.
//
// Every subscription uses events.Coalesce, with one exception:
// notification.raised uses events.Buffer, because a toast is not state the
// frontend re-reads on wake-up — it is the message itself, so coalescing it
// would mean a notification arriving right behind a busier one is simply
// never seen. Every other subscriber only needs the latest state, and a busy
// webview must never hold up the producer goroutine that published.
//
// This is where the core's typed payload is deliberately degraded. Wails
// events are wake-up signals by design — an adapter that needs the delta gets
// it from the bus instead.
func Subscribe(ctx context.Context, bus *events.Bus, onTrayStale func()) (cancel func()) {
	trayStale := func() {
		if onTrayStale != nil {
			onTrayStale()
		}
	}
	cancels := []func(){
		events.Subscribe(ctx, bus, "wailsui.log", events.Coalesce(), func(_ context.Context, e events.LogAppended) {
			emitLogAppended(e.NextOffset)
		}),
		events.Subscribe(ctx, bus, "wailsui.inbox", events.Coalesce(), func(context.Context, events.InboxUpdated) {
			emitInboxUpdated()
			trayStale()
		}),
		events.Subscribe(ctx, bus, "wailsui.activity", events.Coalesce(), func(_ context.Context, e events.ActivityAppended) {
			emitActivityAppended(e.ID)
		}),
		events.Subscribe(ctx, bus, "wailsui.jobs", events.Coalesce(), func(context.Context, events.JobsUpdated) {
			// The core carries the job id; the frontend re-reads the job list,
			// so this is the degradation the wake-up contract asks for.
			emitJobsUpdated()
		}),
		events.Subscribe(ctx, bus, "wailsui.session-create-failed", events.Coalesce(), func(_ context.Context, e events.SessionCreateFailed) {
			emitSessionCreateFailed(e.Name)
		}),
		events.Subscribe(ctx, bus, "wailsui.sessions", events.Coalesce(), func(context.Context, events.SessionsUpdated) {
			emitSessionsUpdated()
		}),
		events.Subscribe(ctx, bus, "wailsui.tasks", events.Coalesce(), func(context.Context, events.TasksUpdated) {
			emitTasksUpdated()
		}),
		events.Subscribe(ctx, bus, "wailsui.actions", events.Coalesce(), func(context.Context, events.ActionsUpdated) {
			emitActionsUpdated()
			trayStale()
		}),
		events.Subscribe(ctx, bus, "wailsui.menu-bar", events.Coalesce(), func(context.Context, events.MenuBarUpdated) {
			trayStale()
		}),
		events.Subscribe(ctx, bus, "wailsui.canvas", events.Coalesce(), func(_ context.Context, e events.CanvasUpdated) {
			emitCanvasUpdated(CanvasWrite{Session: e.Session, HiveSession: e.HiveSession})
		}),
		events.Subscribe(ctx, bus, "wailsui.canvas-toggle", events.Coalesce(), func(_ context.Context, e events.CanvasToggleRequested) {
			emitCanvasToggle(CanvasToggle{Session: e.Session, HiveSession: e.HiveSession, Name: e.Name, Open: e.Open})
		}),
		events.Subscribe(ctx, bus, "wailsui.schedules", events.Coalesce(), func(_ context.Context, e events.SchedulesUpdated) {
			emitSchedulesUpdated(e.Workspace)
		}),
		events.Subscribe(ctx, bus, "wailsui.connection", events.Coalesce(), func(_ context.Context, e events.ConnectionUpdated) {
			emitConnectionUpdated(e.Provider)
		}),
		events.Subscribe(ctx, bus, "wailsui.flows", events.Coalesce(), func(context.Context, events.FlowsUpdated) {
			emitFlowsUpdated()
			trayStale()
		}),
		// A notify terminal's delivery is not state to re-read: it is the
		// message, so every one gets a slot in the queue rather than risking
		// a coalesced drop. InApp is the only one this adapter acts on here —
		// an OS banner already went out through the notifier port itself by
		// the time this publishes (see app.go's observedNotifier).
		events.Subscribe(ctx, bus, "wailsui.notification", events.Buffer(32), func(_ context.Context, e events.NotificationRaised) {
			if !e.InApp {
				return
			}
			emitNotificationToast(NotificationToast{
				Title:    e.Title,
				Body:     e.Body,
				Severity: e.Severity,
			})
		}),
	}
	return func() {
		for _, c := range cancels {
			c()
		}
	}
}

// emitLogAppended pushes the pipeline event log's new tail offset to the
// frontend after a producer tick appends at least one row. Safe to call
// from the producer goroutine once the app is running.
func emitLogAppended(nextOffset int64) {
	if app := application.Get(); app != nil {
		app.Event.Emit("log:appended", nextOffset)
	}
}

// emitInboxUpdated wakes the feed views after the flow engine committed a run:
// membership claims, queued actions and node-run metrics may all have changed.
func emitInboxUpdated() {
	if app := application.Get(); app != nil {
		app.Event.Emit("inbox:updated", "changed")
	}
}

// emitActivityAppended pushes the activity:appended wake-up (carrying the new
// event's id) to the frontend after any subsystem records an activity event.
// Safe to call from any goroutine once the app is running.
func emitActivityAppended(id int64) {
	if app := application.Get(); app != nil {
		app.Event.Emit("activity:appended", id)
	}
}

// CanvasWrite is the canvas:updated payload: the author of the write, a chat
// (Session) or a hive session (HiveSession), never both.
type CanvasWrite struct {
	Session     int64  `json:"session"`
	HiveSession string `json:"hiveSession"`
}

// emitCanvasUpdated pushes the canvas:updated wake-up to the frontend after
// an agent's canvas write. Safe to call from any goroutine once the app is
// running.
func emitCanvasUpdated(write CanvasWrite) {
	if app := application.Get(); app != nil {
		app.Event.Emit("canvas:updated", write)
	}
}

// CanvasToggle is the canvas:toggle payload: whose pane to open or close, a
// chat's or a hive session's, and the canvas to pin when opening (empty
// leaves the pane's pick).
type CanvasToggle struct {
	Session     int64  `json:"session"`
	HiveSession string `json:"hiveSession"`
	Name        string `json:"name"`
	Open        bool   `json:"open"`
}

func emitCanvasToggle(toggle CanvasToggle) {
	if app := application.Get(); app != nil {
		app.Event.Emit("canvas:toggle", toggle)
	}
}

// emitSchedulesUpdated wakes the Chats area after a workspace's scheduled
// chats changed, naming the workspace.
func emitSchedulesUpdated(workspace string) {
	if app := application.Get(); app != nil {
		app.Event.Emit("schedules:updated", workspace)
	}
}

// emitJobsUpdated wakes frontend consumers after any successful job lifecycle
// transition. The payload is intentionally only a wake-up signal.
func emitJobsUpdated() {
	if app := application.Get(); app != nil {
		app.Event.Emit("jobs:updated", "changed")
	}
}

// emitSessionCreateFailed wakes the New Session flow after a create failed.
func emitSessionCreateFailed(name string) {
	if app := application.Get(); app != nil {
		app.Event.Emit("sessions:create-failed", name)
	}
}

func emitSessionsUpdated() {
	if app := application.Get(); app != nil {
		app.Event.Emit("sessions:updated", "changed")
	}
}

func emitTasksUpdated() {
	if app := application.Get(); app != nil {
		app.Event.Emit("tasks:updated", "changed")
	}
}

// MenuBarNavigation routes Settings before ItemID, and ItemID before FeedID.
type MenuBarNavigation struct {
	ProfileID string `json:"profileId"`
	FeedID    string `json:"feedId"`
	ItemID    int64  `json:"itemId"`
	Settings  bool   `json:"settings"`
}

func emitMenuBarOpen(nav MenuBarNavigation) {
	if app := application.Get(); app != nil {
		app.Event.Emit("menubar:open", nav)
	}
}

// emitNotificationActivated tells the frontend which item a clicked
// notification came from, so it can route to it.
func emitNotificationActivated(activation NotificationActivation) {
	if app := application.Get(); app != nil {
		app.Event.Emit("notification:activated", activation)
	}
}

// NotificationActivationFrom reads the item a clicked notification was sent
// about out of the payload the notify executor attached to it. The user info
// makes a native round trip, so its numbers come back in whatever shape the
// platform's serialization chose — hence the tolerant decode. App-level
// notifications carry no such payload and report false.
func NotificationActivationFrom(result wailsnotify.NotificationResult) (NotificationActivation, bool) {
	profileID, _ := result.Response.UserInfo["profileId"].(string)
	if profileID == "" {
		return NotificationActivation{}, false
	}
	activation := NotificationActivation{ProfileID: profileID}
	switch id := result.Response.UserInfo["itemId"].(type) {
	case float64:
		activation.ItemID = int64(id)
	case int64:
		activation.ItemID = id
	case int:
		activation.ItemID = int64(id)
	case json.Number:
		activation.ItemID, _ = id.Int64()
	case string:
		activation.ItemID, _ = strconv.ParseInt(id, 10, 64)
	}
	return activation, true
}

// emitWindowFocus pushes the current focused state to the frontend. Safe to
// call from native window event callbacks once the app is running.
func emitWindowFocus() {
	if app := application.Get(); app != nil {
		app.Event.Emit("window:focus", true)
	}
}

// emitWindowBlur pushes the current unfocused state to the frontend. Safe to
// call from native window event callbacks once the app is running.
func emitWindowBlur() {
	if app := application.Get(); app != nil {
		app.Event.Emit("window:blur", false)
	}
}

// emitConnectionUpdated pushes the connection:updated wake-up to the
// frontend, naming the provider whose credentials changed. Safe to call from
// any goroutine once the app is running.
func emitConnectionUpdated(provider string) {
	if app := application.Get(); app != nil {
		app.Event.Emit("connection:updated", provider)
	}
}

// emitFlowsUpdated pushes the flows:updated wake-up to the frontend. Safe to
// call from any goroutine once the app is running.
func emitFlowsUpdated() {
	if app := application.Get(); app != nil {
		app.Event.Emit("flows:updated", "changed")
	}
}

// emitActionsUpdated wakes frontend consumers after a successful catalog
// change or a watcher reload. Service mutations call it only after success.
func emitActionsUpdated() {
	if app := application.Get(); app != nil {
		app.Event.Emit("actions:updated", "changed")
	}
}

// emitNotificationToast hands a flow notification to the frontend to surface
// in-app. Called from the notification.raised subscription above once the
// core has published; before the app is running (or in a headless build) it
// is a no-op, which matches the native path's own behavior when
// notifications are unavailable.
//
// A var, not a func — same seam as emitUpdateAvailable below — so a test can
// swap it for a spy without a running Wails application.
var emitNotificationToast = func(toast NotificationToast) {
	if app := application.Get(); app != nil {
		app.Event.Emit("notification:toast", toast)
	}
}

// emitUpdateAvailable pushes update:available, carrying the latest
// UpdateInfo, when a self-update check finds a newer desktop release; the
// title bar reacts to it. It is not reached through the core's event bus —
// update checking is adapter-owned end to end (see UpdaterService) — so it
// is called directly from there rather than from a Subscribe handler above;
// it lives here only so every emission this adapter makes is registered and
// emitted from this one file.
var emitUpdateAvailable = func(info UpdateInfo) {
	if app := application.Get(); app != nil {
		app.Event.Emit("update:available", info)
	}
}

type TerminalFilesDropped struct {
	Target string   `json:"target"`
	Paths  []string `json:"paths"`
}

func emitTerminalFilesDropped(drop TerminalFilesDropped) {
	if app := application.Get(); app != nil {
		app.Event.Emit("terminal:files-dropped", drop)
	}
}
