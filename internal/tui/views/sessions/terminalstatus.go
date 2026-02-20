package sessions

import (
	"context"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/rs/zerolog/log"

	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/core/terminal"
)

const terminalStatusTimeout = 2 * time.Second

// WindowStatus holds per-window terminal status for multi-window sessions.
type WindowStatus struct {
	WindowIndex string
	WindowName  string
	Status      terminal.Status
	Tool        string
	PaneContent string
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

// allWindowsDiscoverer is implemented by integrations that can return all windows.
type allWindowsDiscoverer interface {
	DiscoverAllWindows(ctx context.Context, slug string, metadata map[string]string) ([]*terminal.SessionInfo, error)
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
		for k, v := range sess.Metadata {
			metadata[k] = v
		}
		metadata["_session_path"] = sess.Path
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

	// Discover all windows if the integration supports it.
	if disc, ok := integration.(allWindowsDiscoverer); ok {
		allInfos, discErr := disc.DiscoverAllWindows(ctx, sess.Slug, metadata)
		if discErr != nil {
			log.Debug().Err(discErr).Str("session", sess.Slug).Msg("multi-window discovery failed, using single-window mode")
		} else if len(allInfos) > 1 {
			windows := make([]WindowStatus, 0, len(allInfos))
			for _, wi := range allInfos {
				// Get per-window status and content
				wStatus, wErr := integration.GetStatus(ctx, wi)
				if wErr != nil {
					log.Debug().Err(wErr).Str("session", sess.Slug).Str("window", wi.WindowIndex).Msg("per-window status failed, marking missing")
					wStatus = terminal.StatusMissing
				}
				windows = append(windows, WindowStatus{
					WindowIndex: wi.WindowIndex,
					WindowName:  wi.WindowName,
					Status:      wStatus,
					Tool:        wi.DetectedTool,
					PaneContent: wi.PaneContent,
				})
			}
			if len(windows) > 1 {
				status.Windows = windows
			}
		}
	}

	return status
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
