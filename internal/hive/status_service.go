package hive

import (
	"context"
	"maps"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/core/terminal"
	terminaltmux "github.com/colonyops/hive/internal/core/terminal/tmux"
)

const terminalStatusTimeout = 2 * time.Second

// PaneStatus holds per-pane terminal status for agent panes.
type PaneStatus struct {
	PaneID      string
	Status      terminal.Status
	Tool        string
	PaneContent string
	IsAgent     bool
}

// WindowStatus holds per-window terminal status for multi-window sessions.
type WindowStatus struct {
	WindowIndex string
	WindowName  string
	Status      terminal.Status
	Tool        string
	PaneContent string
	Panes       []PaneStatus
}

// TerminalStatus holds the terminal integration status for a session.
type TerminalStatus struct {
	Status      terminal.Status
	Tool        string
	WindowName  string
	PaneContent string
	IsLoading   bool
	Error       error
	Windows     []WindowStatus // per-window statuses (populated only for multi-window sessions)
}

// StatusService performs agent status detection for sessions via terminal
// integrations. It is UI-agnostic so embedders can run status checks outside
// the TUI.
type StatusService struct {
	term    *terminal.Manager
	workers int
}

// NewStatusService creates a StatusService. workers bounds the number of
// concurrent per-session status fetches in FetchBatch.
func NewStatusService(term *terminal.Manager, workers int) *StatusService {
	if workers < 1 {
		workers = 1
	}
	return &StatusService{term: term, workers: workers}
}

// Available reports whether any terminal integration is enabled and usable.
// It is safe to call on a nil service.
func (s *StatusService) Available() bool {
	return s != nil && s.term != nil && s.term.HasEnabledIntegrations()
}

// RootRepoTarget identifies a workspace checkout to poll for agent status.
// Name doubles as the tmux session slug because opening a repo header names
// the root repo's tmux session after the repo name.
type RootRepoTarget struct {
	Name string
	Path string
}

// RootStatusKey returns the FetchBatch result key for a root checkout.
// Prefixed so it can never collide with session IDs, which share the map.
func RootStatusKey(path string) string {
	return "root:" + path
}

// FetchBatch fetches terminal status for the given sessions and workspace
// root checkouts concurrently. Results are keyed by session ID for sessions
// and by RootStatusKey for roots. Non-active sessions are skipped. Each
// per-target fetch is bounded by an internal timeout in addition to ctx.
//
// Roots must share the batch rather than run as a separate call: the tmux
// integration only serves discovery from a cache younger than 2s, so a
// separate call would race the RefreshAll here and see a stale cache,
// silently missing statuses.
func (s *StatusService) FetchBatch(ctx context.Context, sessions []*session.Session, roots []RootRepoTarget) map[string]TerminalStatus {
	results := make(map[string]TerminalStatus)
	if (len(sessions) == 0 && len(roots) == 0) || !s.Available() {
		return results
	}

	// Refresh integration caches once before fetching statuses
	s.term.RefreshAll()

	var mu sync.Mutex
	sem := make(chan struct{}, s.workers)
	var wg sync.WaitGroup

	for _, sess := range sessions {
		if sess.State != session.StateActive {
			continue
		}

		wg.Add(1)
		go func(sess *session.Session) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			fetchCtx, cancel := context.WithTimeout(ctx, terminalStatusTimeout)
			defer cancel()

			status := s.FetchSession(fetchCtx, sess)

			mu.Lock()
			results[sess.ID] = status
			mu.Unlock()
		}(sess)
	}

	for _, target := range roots {
		wg.Add(1)
		go func(rt RootRepoTarget) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			fetchCtx, cancel := context.WithTimeout(ctx, terminalStatusTimeout)
			defer cancel()

			status := s.fetchRoot(fetchCtx, rt)

			mu.Lock()
			results[RootStatusKey(rt.Path)] = status
			mu.Unlock()
		}(target)
	}

	wg.Wait()
	return results
}

// fetchRoot fetches terminal status for a workspace root checkout. Unlike
// sessions, root repos don't expand window sub-items, so multi-window
// discovery is skipped.
func (s *StatusService) fetchRoot(ctx context.Context, target RootRepoTarget) TerminalStatus {
	status := TerminalStatus{Status: terminal.StatusMissing}

	metadata := map[string]string{terminaltmux.SessionPathKey: target.Path}
	info, integration, err := s.term.DiscoverSession(ctx, target.Name, metadata)
	if err != nil {
		log.Debug().Err(err).Str("repo", target.Name).Msg("root repo terminal discovery failed")
		status.Error = err
		return status
	}
	if info == nil || integration == nil {
		return status
	}

	termStatus, err := integration.GetStatus(ctx, info)
	if err != nil {
		log.Debug().Err(err).Str("repo", target.Name).Msg("root repo terminal status lookup failed")
		status.Error = err
		return status
	}

	status.Status = termStatus
	status.Tool = info.DetectedTool
	status.WindowName = info.WindowName
	status.PaneContent = info.PaneContent
	return status
}

// FetchSession fetches terminal status for a single session.
func (s *StatusService) FetchSession(ctx context.Context, sess *session.Session) TerminalStatus {
	status := TerminalStatus{
		Status: terminal.StatusMissing,
	}

	// Inject session path into metadata for multi-window disambiguation
	metadata := sess.Metadata
	if sess.Path != "" {
		metadata = make(map[string]string, len(sess.Metadata)+1)
		maps.Copy(metadata, sess.Metadata)
		metadata[terminaltmux.SessionPathKey] = sess.Path
	}

	// Try to discover terminal session
	info, integration, err := s.term.DiscoverSession(ctx, sess.Slug, metadata)
	if err != nil {
		log.Debug().Err(err).Str("session", sess.Slug).Msg("terminal session discovery failed")
		status.Error = err
		return status
	}

	if info == nil || integration == nil {
		return status
	}

	// Get status from integration
	termStatus, err := integration.GetStatus(ctx, info)
	if err != nil {
		log.Debug().Err(err).Str("session", sess.Slug).Msg("terminal status lookup failed")
		status.Error = err
		return status
	}

	status.Status = termStatus
	status.Tool = info.DetectedTool
	status.WindowName = info.WindowName
	status.PaneContent = info.PaneContent

	// Discover all panes/windows if the integration supports it.
	var allInfos []*terminal.SessionInfo
	var discErr error
	if disc, ok := integration.(terminal.AllPanesDiscoverer); ok {
		allInfos, discErr = disc.DiscoverAllPanes(ctx, sess.Slug, metadata)
	}
	if allInfos != nil || discErr != nil {
		if discErr != nil {
			log.Debug().Err(discErr).Str("session", sess.Slug).Msg("multi-window discovery failed, using single-window mode")
		} else if len(allInfos) > 0 {
			windows := groupPaneStatuses(ctx, integration, sess.Slug, allInfos)
			if ShouldExposeWindows(windows) {
				status.Windows = windows
			}
		}
	}

	return status
}

func groupPaneStatuses(ctx context.Context, integration terminal.Integration, slug string, infos []*terminal.SessionInfo) []WindowStatus {
	windows := make([]WindowStatus, 0, len(infos))
	byWindow := make(map[string]int, len(infos))
	for _, wi := range infos {
		paneStatus, wErr := integration.GetStatus(ctx, wi)
		if wErr != nil {
			log.Debug().Err(wErr).Str("session", slug).Str("window", wi.WindowIndex).Str("pane", wi.PaneID).Msg("per-pane status failed, marking missing")
			paneStatus = terminal.StatusMissing
		}

		pane := PaneStatus{
			PaneID:      wi.PaneID,
			Status:      paneStatus,
			Tool:        wi.DetectedTool,
			PaneContent: wi.PaneContent,
			IsAgent:     true,
		}

		// \x1f is an ASCII Unit Separator, which avoids collisions with printable tmux window names.
		key := wi.WindowIndex + "\x1f" + wi.WindowName
		idx, ok := byWindow[key]
		if !ok {
			idx = len(windows)
			byWindow[key] = idx
			windows = append(windows, WindowStatus{
				WindowIndex: wi.WindowIndex,
				WindowName:  wi.WindowName,
				Status:      paneStatus,
				Tool:        wi.DetectedTool,
				PaneContent: wi.PaneContent,
			})
		} else {
			windows[idx].Status = aggregateStatus(windows[idx].Status, paneStatus)
			if windows[idx].Tool == "" {
				windows[idx].Tool = wi.DetectedTool
			}
			if windows[idx].PaneContent == "" {
				windows[idx].PaneContent = wi.PaneContent
			}
		}
		windows[idx].Panes = append(windows[idx].Panes, pane)
	}
	return windows
}

// ShouldExposeWindows reports whether a session's windows warrant per-window
// breakdown: more than one window, or a single window with multiple agent panes.
func ShouldExposeWindows(windows []WindowStatus) bool {
	if len(windows) > 1 {
		return true
	}
	return len(windows) == 1 && len(windows[0].Panes) > 1
}

func aggregateStatus(current, next terminal.Status) terminal.Status {
	if statusRank(next) > statusRank(current) {
		return next
	}
	return current
}

// statusRank ranks missing above ready because a broken or missing pane in a
// window is more actionable than a ready sibling — aggregation must surface
// it, not mask it behind a sibling that happens to be idle. Question ranks
// directly below approval since both block on the user and question renders
// at the approval tier.
func statusRank(status terminal.Status) int {
	switch status {
	case terminal.StatusApproval:
		return 5
	case terminal.StatusQuestion:
		return 4
	case terminal.StatusActive:
		return 3
	case terminal.StatusMissing:
		return 2
	case terminal.StatusReady:
		return 1
	default:
		return 0
	}
}
