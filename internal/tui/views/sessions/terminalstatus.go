package sessions

import (
	"context"
	"maps"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/colonyops/hive/internal/core/config"

	tea "charm.land/bubbletea/v2"
	"github.com/rs/zerolog/log"

	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/core/terminal"
	terminaltmux "github.com/colonyops/hive/internal/core/terminal/tmux"
)

const terminalStatusTimeout = 2 * time.Second

// resolvedHostname caches os.Hostname() so the per-pane hot path avoids
// repeated syscalls. Hostname is constant for the lifetime of the process.
var (
	hostnameOnce     sync.Once
	resolvedHostname string
)

func getHostname() string {
	hostnameOnce.Do(func() {
		h, _ := os.Hostname()
		resolvedHostname = strings.ToLower(strings.TrimSpace(h))
	})
	return resolvedHostname
}

// PaneStatus holds per-pane terminal status for agent panes.
type PaneStatus struct {
	PaneID      string
	Status      terminal.Status
	Tool        string
	PaneContent string
	IsAgent     bool
	Ports       []int
}

// WindowStatus holds per-window terminal status for multi-window sessions.
type WindowStatus struct {
	WindowIndex string
	WindowName  string
	Status      terminal.Status
	Tool        string
	PaneContent string
	HasAgent    bool
	Ports       []int
	Panes       []PaneStatus
}

// TerminalStatus holds the terminal integration status for a session.
type TerminalStatus struct {
	Status      terminal.Status
	Tool        string
	WindowName  string
	PaneContent string
	Ports       []int
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
func FetchTerminalStatusBatch(mgr *terminal.Manager, sessions []*session.Session, workers int, tmuxItems string) tea.Cmd {
	if len(sessions) == 0 || !mgr.HasEnabledIntegrations() {
		return nil
	}
	if workers <= 0 {
		workers = 1
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

				status := fetchTerminalStatusForSession(ctx, mgr, s, tmuxItems)

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
func fetchTerminalStatusForSession(ctx context.Context, mgr *terminal.Manager, sess *session.Session, tmuxItems string) TerminalStatus {
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
		if tmuxItems == config.TmuxItemsAll {
			windows := discoverAllWindows(ctx, mgr, sess.Slug, metadata)
			if len(windows) > 0 {
				status.Status = terminal.StatusNeutral
				status.Ports = portsForWindows(windows)
				status.Windows = windows
			}
		}
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
	status.Ports = append([]int(nil), info.Ports...)

	// Discover all panes/windows if the integration supports it.
	var allInfos []*terminal.SessionInfo
	var discErr error
	if disc, ok := integration.(terminal.AllPanesDiscoverer); ok {
		allInfos, discErr = disc.DiscoverAllPanes(ctx, sess.Slug, metadata, tmuxItems == config.TmuxItemsAll)
	}
	if allInfos != nil || discErr != nil {
		if discErr != nil {
			log.Debug().Err(discErr).Str("session", sess.Slug).Msg("multi-window discovery failed, using single-window mode")
		} else if len(allInfos) > 0 {
			windows := groupPaneStatuses(ctx, integration, sess.Slug, allInfos)
			status.Ports = mergePorts(status.Ports, portsForWindows(windows))
			if shouldExposeWindowsForMode(windows, tmuxItems) {
				status.Windows = windows
			}
		}
	}

	return status
}

func discoverAllWindows(ctx context.Context, mgr *terminal.Manager, slug string, metadata map[string]string) []WindowStatus {
	for _, integration := range mgr.EnabledIntegrations() {
		disc, ok := integration.(terminal.AllPanesDiscoverer)
		if !ok {
			continue
		}
		infos, err := disc.DiscoverAllPanes(ctx, slug, metadata, true)
		if err != nil {
			log.Debug().Err(err).Str("session", slug).Msg("all tmux item discovery failed")
			continue
		}
		windows := groupPaneStatuses(ctx, integration, slug, infos)
		if len(windows) > 0 {
			return windows
		}
	}
	return nil
}

func groupPaneStatuses(ctx context.Context, integration terminal.Integration, slug string, infos []*terminal.SessionInfo) []WindowStatus {
	windows := make([]WindowStatus, 0, len(infos))
	byWindow := make(map[string]int, len(infos))
	for _, wi := range infos {
		paneStatus := terminal.Status("")
		if wi.IsAgent {
			var wErr error
			paneStatus, wErr = integration.GetStatus(ctx, wi)
			if wErr != nil {
				log.Debug().Err(wErr).Str("session", slug).Str("window", wi.WindowIndex).Str("pane", wi.PaneID).Msg("per-pane status failed, marking missing")
				paneStatus = terminal.StatusMissing
			}
		}

		pane := PaneStatus{
			PaneID:      wi.PaneID,
			Status:      paneStatus,
			Tool:        paneLabel(wi),
			PaneContent: wi.PaneContent,
			IsAgent:     wi.IsAgent,
			Ports:       append([]int(nil), wi.Ports...),
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
				HasAgent:    wi.IsAgent,
				Ports:       append([]int(nil), wi.Ports...),
			})
		} else {
			windows[idx].Ports = mergePorts(windows[idx].Ports, wi.Ports)
			if wi.IsAgent {
				windows[idx].Status = aggregateStatus(windows[idx].Status, paneStatus)
				windows[idx].HasAgent = true
				if windows[idx].Tool == "" {
					windows[idx].Tool = wi.DetectedTool
				}
				if windows[idx].PaneContent == "" {
					windows[idx].PaneContent = wi.PaneContent
				}
			}
		}
		windows[idx].Panes = append(windows[idx].Panes, pane)
	}
	return windows
}

// shouldExposeWindowsForMode reports whether the windows list should be
// expanded into child rows in the session tree.
//
// In "agents" mode (default), windows are only shown when there are multiple
// windows or multiple panes within a single window — the agent must be
// identifiable among siblings. In "all" mode the same threshold applies
// because a single-pane, single-window session has no hierarchy to show.
func shouldExposeWindowsForMode(windows []WindowStatus, tmuxItems string) bool {
	_ = tmuxItems // both modes share the same threshold today
	if len(windows) == 0 {
		return false
	}
	if len(windows) > 1 {
		return true
	}
	return len(windows[0].Panes) > 1
}

func portsForWindows(windows []WindowStatus) []int {
	var ports []int
	for _, window := range windows {
		ports = mergePorts(ports, window.Ports)
		for _, pane := range window.Panes {
			ports = mergePorts(ports, pane.Ports)
		}
	}
	return ports
}

func mergePorts(a, b []int) []int {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	seen := make(map[int]bool, len(a)+len(b))
	merged := make([]int, 0, len(a)+len(b))
	for _, port := range append(append([]int(nil), a...), b...) {
		if port <= 0 || seen[port] {
			continue
		}
		seen[port] = true
		merged = append(merged, port)
	}
	slices.Sort(merged)
	return merged
}

// paneLabel returns the display label for a pane.
// Priority: detected AI tool > custom pane title (non-default) > window name > "terminal".
// Default pane titles (hostname, ".local" suffix) are suppressed so users see
// meaningful names rather than OS-generated noise.
func paneLabel(info *terminal.SessionInfo) string {
	if info == nil {
		return ""
	}
	if info.DetectedTool != "" {
		return info.DetectedTool
	}
	if info.PaneTitle != "" && !isDefaultPaneTitle(info.PaneTitle) {
		return info.PaneTitle
	}
	if info.WindowName != "" {
		return info.WindowName
	}
	return "terminal"
}

// isDefaultPaneTitle reports whether title is the default tmux pane title that
// tmux sets automatically (process name or hostname). These should be suppressed
// in favour of the window name so users see meaningful labels.
func isDefaultPaneTitle(title string) bool {
	title = strings.TrimSpace(title)
	if title == "" {
		return false
	}
	lowerTitle := strings.ToLower(title)
	// tmux often sets the pane title to "hostname.local" on macOS.
	if strings.HasSuffix(lowerTitle, ".local") {
		return true
	}
	lowerHost := getHostname()
	if lowerHost == "" {
		return false
	}
	if lowerTitle == lowerHost {
		return true
	}
	shortHost, _, _ := strings.Cut(lowerHost, ".")
	return shortHost != "" && lowerTitle == shortHost
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
