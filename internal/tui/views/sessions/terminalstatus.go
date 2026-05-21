package sessions

import (
	"context"
	"maps"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
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

// TerminalStatusBatchCompleteMsg is sent when all terminal status fetches complete.
type TerminalStatusBatchCompleteMsg struct {
	Results map[string]TerminalStatus // sessionID -> status
}

// TerminalPollTickMsg triggers a terminal status poll cycle.
type TerminalPollTickMsg struct{}

// FetchTerminalStatusBatch returns a command that fetches terminal status for multiple sessions.
func FetchTerminalStatusBatch(mgr *terminal.Manager, sessions []*session.Session, workers int) tea.Cmd {
	if len(sessions) == 0 || !mgr.HasEnabledIntegrations() {
		return nil
	}

	return func() tea.Msg {
		// Refresh integration caches once before fetching statuses
		mgr.RefreshAll()

		results := make(map[string]TerminalStatus)
		var mu sync.Mutex

		sem := make(chan struct{}, workers)
		var wg sync.WaitGroup

		for _, sess := range sessions {
			// Skip non-active sessions
			if sess.State != session.StateActive {
				continue
			}

			wg.Add(1)
			go func(s *session.Session) {
				defer wg.Done()

				sem <- struct{}{}
				defer func() { <-sem }()

				ctx, cancel := context.WithTimeout(context.Background(), terminalStatusTimeout)
				defer cancel()

				status := fetchTerminalStatusForSession(ctx, mgr, s)

				mu.Lock()
				results[s.ID] = status
				mu.Unlock()
			}(sess)
		}

		wg.Wait()
		return TerminalStatusBatchCompleteMsg{Results: results}
	}
}

// fetchTerminalStatusForSession fetches terminal status for a single session.
func fetchTerminalStatusForSession(ctx context.Context, mgr *terminal.Manager, sess *session.Session) TerminalStatus {
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
	info, integration, err := mgr.DiscoverSession(ctx, sess.Slug, metadata)
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
	} else if disc, ok := integration.(terminal.AllWindowsDiscoverer); ok {
		allInfos, discErr = disc.DiscoverAllWindows(ctx, sess.Slug, metadata)
	}
	if allInfos != nil || discErr != nil {
		if discErr != nil {
			log.Debug().Err(discErr).Str("session", sess.Slug).Msg("multi-window discovery failed, using single-window mode")
		} else if len(allInfos) > 0 {
			windows := groupPaneStatuses(ctx, integration, sess.Slug, allInfos)
			if shouldExposeWindows(windows) {
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

func shouldExposeWindows(windows []WindowStatus) bool {
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

func statusRank(status terminal.Status) int {
	switch status {
	case terminal.StatusApproval:
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

// StartTerminalPollTicker returns a command that starts the terminal status poll ticker.
func StartTerminalPollTicker(interval time.Duration) tea.Cmd {
	if interval <= 0 {
		return nil
	}
	return tea.Tick(interval, func(time.Time) tea.Msg {
		return TerminalPollTickMsg{}
	})
}
