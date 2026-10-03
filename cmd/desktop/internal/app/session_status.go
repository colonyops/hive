package app

import (
	"context"
	"fmt"
	"time"

	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/domain/terminal"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	statussvc "github.com/colonyops/hive/internal/hive/status"
	tmuxcc "github.com/colonyops/hive/internal/platform/tmux/control"
)

// sessionWindowSource joins hive's index-based window status to the stable
// tmux window id every desktop operation uses.
type sessionWindowSource interface {
	ListIndexedWindows(ctx context.Context, sessions []string) (map[string][]tmuxcc.IndexedWindow, error)
}

// SessionWindowStatus is one tmux window's detected agent activity. Status is
// simplified: ready, active, or approval.
type SessionWindowStatus struct {
	WindowID string
	Status   terminal.Status
	Tool     string
}

// SessionStatus separates tmux liveness from the activity detected in each
// agent window.
type SessionStatus struct {
	SessionID string
	Running   bool
	Windows   []SessionWindowStatus
}

// SessionStatusSnapshot carries one poll result and the hive-configured delay
// the caller should use before requesting the next one.
type SessionStatusSnapshot struct {
	Items        []SessionStatus
	PollInterval time.Duration
}

// SessionStatuses detects the live agent state for active sessions. Missing or
// unavailable terminals are data, so only a failure to read the session set
// fails the request.
func (s *SessionsService) SessionStatuses(ctx context.Context) (SessionStatusSnapshot, error) {
	snapshot := SessionStatusSnapshot{
		Items:        []SessionStatus{},
		PollInterval: s.hive.Config().Tmux.PollInterval,
	}
	statuses := s.hive.Status()
	if !statuses.Available() {
		return snapshot, nil
	}

	sessions, err := s.hive.Sessions().ListSessions(ctx)
	if err != nil {
		return SessionStatusSnapshot{}, Wrap(err, KindInternal, "reading session status")
	}
	active := make([]*session.Session, 0, len(sessions))
	for i := range sessions {
		if sessions[i].State == session.StateActive {
			active = append(active, &sessions[i])
		}
	}
	terminalStatuses := statuses.FetchBatch(ctx, active, nil)
	windowSets, err := s.sessionWindows(ctx, active)
	if err != nil {
		return SessionStatusSnapshot{}, Wrap(err, KindInternal, "listing tmux windows for status")
	}
	snapshot.Items = projectSessionStatuses(active, terminalStatuses, windowSets, s.windows != nil)
	return snapshot, nil
}

// projectSessionStatuses maps hive's per-session status onto stable window
// ids. Without a window source, liveness falls back to hive's own detection.
func projectSessionStatuses(
	active []*session.Session,
	statuses map[string]statussvc.TerminalStatus,
	windowSets map[string][]tmuxcc.IndexedWindow,
	haveWindows bool,
) []SessionStatus {
	items := []SessionStatus{}
	for _, s := range active {
		status, ok := statuses[s.ID]
		if !ok {
			continue
		}
		refs := windowSets[sessionsvc.Target(*s).Session]
		item := SessionStatus{SessionID: s.ID, Running: len(refs) > 0, Windows: []SessionWindowStatus{}}
		if !haveWindows {
			item.Running = status.Status != terminal.StatusMissing
		}
		if len(status.Windows) == 0 {
			if windowID := stableWindowID("", status.WindowName, refs); windowID != "" {
				item.Windows = append(item.Windows, SessionWindowStatus{
					WindowID: windowID,
					Status:   status.Status.Simplified(),
					Tool:     status.Tool,
				})
			}
		}
		for _, window := range status.Windows {
			windowID := stableWindowID(window.WindowIndex, window.WindowName, refs)
			if windowID == "" {
				continue
			}
			item.Windows = append(item.Windows, SessionWindowStatus{
				WindowID: windowID,
				Status:   window.Status.Simplified(),
				Tool:     window.Tool,
			})
		}
		items = append(items, item)
	}
	return items
}

// runningSessions reports which of ids currently have a live tmux session. It
// is the narrow counterpart to SessionStatuses: an inbox item asks about the
// one or two sessions it spawned, and a full sweep would pay a tmux round trip
// for every active session in the install to answer that.
//
// An unavailable status source is data, not a failure: nothing is reported
// running, which is what "we cannot see tmux from here" honestly looks like.
func (s *SessionsService) runningSessions(ctx context.Context, sessions []session.Session, ids []string) (map[string]bool, error) {
	running := map[string]bool{}
	statuses := s.hive.Status()
	if len(ids) == 0 || !statuses.Available() {
		return running, nil
	}
	wanted := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		wanted[id] = struct{}{}
	}
	subset := make([]*session.Session, 0, len(ids))
	for i := range sessions {
		if _, ok := wanted[sessions[i].ID]; !ok || sessions[i].State != session.StateActive {
			continue
		}
		subset = append(subset, &sessions[i])
	}
	if s.windows == nil {
		for id, status := range statuses.FetchBatch(ctx, subset, nil) {
			running[id] = status.Status != terminal.StatusMissing
		}
		return running, nil
	}
	windowSets, err := s.sessionWindows(ctx, subset)
	if err != nil {
		return nil, fmt.Errorf("list tmux windows for liveness: %w", err)
	}
	for _, sess := range subset {
		if len(windowSets[sessionsvc.Target(*sess).Session]) > 0 {
			running[sess.ID] = true
		}
	}
	return running, nil
}

func (s *SessionsService) sessionWindows(ctx context.Context, sessions []*session.Session) (map[string][]tmuxcc.IndexedWindow, error) {
	if s.windows == nil {
		return nil, nil
	}
	targets := make([]string, 0, len(sessions))
	for _, sess := range sessions {
		targets = append(targets, sessionsvc.Target(*sess).Session)
	}
	return s.windows.ListIndexedWindows(ctx, targets)
}

// stableWindowID prefers the index, checked against the name, because tmux
// reuses an index once its window closes. A name matches only when exactly one
// window carries it.
func stableWindowID(index, name string, refs []tmuxcc.IndexedWindow) string {
	for _, ref := range refs {
		if index != "" && ref.Index == index && (name == "" || ref.Name == name) {
			return ref.ID
		}
	}
	if name != "" {
		match := ""
		for _, ref := range refs {
			if ref.Name != name {
				continue
			}
			if match != "" {
				return ""
			}
			match = ref.ID
		}
		if match != "" {
			return match
		}
	}
	if index == "" && len(refs) == 1 {
		return refs[0].ID
	}
	return ""
}
