// Package tmux implements terminal integration for tmux.
package tmux

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/colonyops/hive/internal/core/terminal/process"
)

// Integration implements terminal.Integration for tmux.
type Integration struct {
	mu               sync.RWMutex
	cache            map[string]*sessionCache
	cacheTime        time.Time
	trackers         map[string]*terminal.StateTracker
	limiters         map[string]*terminal.RateLimiter
	available        bool
	availableOnce    sync.Once
	preferredWindows []*regexp.Regexp // compiled patterns for preferred window names
}

// agentWindow holds per-window state within a tmux session.
type agentWindow struct {
	windowIndex       string   // window index for targeted capture (e.g., "0", "1")
	windowName        string   // window name for display and matching
	primaryPaneID     string   // pane tagged with @hive-session, or first pane
	panePID           int64    // PID of the primary pane
	allPaneIDs        []string // all pane IDs in this window
	workDir           string
	activity          int64
	paneContent       string
	lastCaptureActive int64
	cachedStatus      terminal.Status
}

// sessionCache holds all tracked windows for a single tmux session.
type sessionCache struct {
	agentWindows []*agentWindow
}

// findWindow returns the agentWindow matching the given window index, or nil.
func (sc *sessionCache) findWindow(windowIndex string) *agentWindow {
	for _, w := range sc.agentWindows {
		if w.windowIndex == windowIndex {
			return w
		}
	}
	return nil
}

// bestWindow returns the highest-activity window, or nil if empty.
func (sc *sessionCache) bestWindow() *agentWindow {
	var best *agentWindow
	for _, w := range sc.agentWindows {
		if best == nil || w.activity > best.activity {
			best = w
		}
	}
	return best
}

// New creates a new tmux integration with optional preferred window patterns.
// Patterns are regex strings matched against window names (e.g., "claude", "aider").
func New(preferredWindowPatterns []string) *Integration {
	var patterns []*regexp.Regexp
	for _, p := range preferredWindowPatterns {
		re, err := regexp.Compile("(?i)" + p) // case-insensitive
		if err != nil {
			log.Warn().Str("pattern", p).Err(err).Msg("invalid preferred window pattern, skipping")
			continue
		}
		patterns = append(patterns, re)
	}

	return &Integration{
		cache:            make(map[string]*sessionCache),
		trackers:         make(map[string]*terminal.StateTracker),
		limiters:         make(map[string]*terminal.RateLimiter),
		preferredWindows: patterns,
	}
}

// Name returns "tmux".
func (t *Integration) Name() string {
	return "tmux"
}

// Available returns true if tmux is installed and accessible.
func (t *Integration) Available() bool {
	t.availableOnce.Do(func() {
		_, err := exec.LookPath("tmux")
		t.available = err == nil
	})
	return t.available
}

// RefreshCache updates the cached session list. Call once per poll cycle.
func (t *Integration) RefreshCache() {
	cmd := exec.Command("tmux", "list-panes", "-a", "-F",
		"#{session_name}\t#{window_index}\t#{window_name}\t#{pane_current_path}\t#{window_activity}\t#{pane_id}\t#{pane_pid}\t#{@hive-session}")
	output, err := cmd.Output()
	if err != nil {
		log.Debug().Err(err).Msg("tmux list-panes failed, clearing cache")
		t.mu.Lock()
		t.cache = make(map[string]*sessionCache)
		t.cacheTime = time.Time{}
		t.mu.Unlock()
		return
	}

	// Snapshot old window state under lock so we can safely copy into new entries.
	type windowSnapshot struct {
		paneContent       string
		lastCaptureActive int64
		cachedStatus      terminal.Status
	}
	t.mu.RLock()
	oldWindows := make(map[string]windowSnapshot) // key: "session:windowIndex"
	for name, sc := range t.cache {
		for _, w := range sc.agentWindows {
			oldWindows[name+":"+w.windowIndex] = windowSnapshot{
				paneContent:       w.paneContent,
				lastCaptureActive: w.lastCaptureActive,
				cachedStatus:      w.cachedStatus,
			}
		}
	}
	t.mu.RUnlock()

	// paneRecord holds per-pane data from a single list-panes line.
	type paneRecord struct {
		paneID      string
		panePID     int64
		hiveSession string // value of @hive-session option, empty if untagged
	}
	// windowAccum accumulates panes that share the same tmux session+window.
	type windowAccum struct {
		windowName string
		workDir    string
		activity   int64
		panes      []paneRecord
	}

	// First pass: collect panes grouped by "sessionName:windowIndex".
	type windowKey struct{ session, index string }
	accumMap := make(map[windowKey]*windowAccum)
	var order []windowKey // preserve insertion order for deterministic output

	for line := range strings.SplitSeq(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 8)
		if len(parts) < 6 {
			continue
		}
		sessName := parts[0]
		winIdx := parts[1]
		winName := parts[2]
		workDir := parts[3]
		paneID := parts[5]

		key := windowKey{sessName, winIdx}
		acc := accumMap[key]
		if acc == nil {
			acc = &windowAccum{windowName: winName, workDir: workDir}
			_, _ = fmt.Sscanf(parts[4], "%d", &acc.activity)
			accumMap[key] = acc
			order = append(order, key)
		}

		rec := paneRecord{paneID: paneID}
		if len(parts) >= 7 {
			_, _ = fmt.Sscanf(parts[6], "%d", &rec.panePID)
		}
		if len(parts) >= 8 {
			rec.hiveSession = strings.TrimSpace(parts[7])
		}
		acc.panes = append(acc.panes, rec)
	}

	// Second pass: convert accumulators to agentWindow, grouped by tmux session.
	type windowEntry struct {
		window    *agentWindow
		preferred bool
	}
	collected := make(map[string][]windowEntry)

	for _, key := range order {
		acc := accumMap[key]

		// Pick primary pane: first pane tagged with @hive-session, else first pane.
		var primaryPaneID string
		var panePID int64
		allPaneIDs := make([]string, 0, len(acc.panes))
		for _, p := range acc.panes {
			allPaneIDs = append(allPaneIDs, p.paneID)
			if primaryPaneID == "" && p.hiveSession != "" {
				primaryPaneID = p.paneID
				panePID = p.panePID
			}
		}
		if primaryPaneID == "" && len(acc.panes) > 0 {
			primaryPaneID = acc.panes[0].paneID
			panePID = acc.panes[0].panePID
		}

		w := &agentWindow{
			windowIndex:   key.index,
			windowName:    acc.windowName,
			primaryPaneID: primaryPaneID,
			panePID:       panePID,
			allPaneIDs:    allPaneIDs,
			workDir:       acc.workDir,
			activity:      acc.activity,
		}

		// Carry over cached content
		if snap, ok := oldWindows[key.session+":"+key.index]; ok {
			w.paneContent = snap.paneContent
			w.lastCaptureActive = snap.lastCaptureActive
			w.cachedStatus = snap.cachedStatus
		}

		isPreferred := t.matchesPreferredWindow(acc.windowName)
		collected[key.session] = append(collected[key.session], windowEntry{window: w, preferred: isPreferred})
	}

	// Third pass: same preferred-window filtering as before.
	newCache := make(map[string]*sessionCache, len(collected))
	for sessionName, entries := range collected {
		hasPreferred := false
		for _, e := range entries {
			if e.preferred {
				hasPreferred = true
				break
			}
		}
		sc := &sessionCache{}
		if hasPreferred {
			for _, e := range entries {
				if e.preferred {
					sc.agentWindows = append(sc.agentWindows, e.window)
				}
			}
		} else {
			var best *agentWindow
			for _, e := range entries {
				if best == nil || e.window.activity > best.activity {
					best = e.window
				}
			}
			if best != nil {
				sc.agentWindows = []*agentWindow{best}
			}
		}
		newCache[sessionName] = sc
	}

	t.mu.Lock()
	t.cache = newCache
	t.cacheTime = time.Now()
	t.mu.Unlock()
}

// matchesPreferredWindow returns true if windowName matches any preferred pattern.
func (t *Integration) matchesPreferredWindow(windowName string) bool {
	for _, re := range t.preferredWindows {
		if re.MatchString(windowName) {
			return true
		}
	}
	return false
}

// sessionPathKey is the metadata key injected by the TUI to pass session path for multi-window matching.
const sessionPathKey = "_session_path"

// findSessionCache locates the sessionCache for a slug using metadata or exact match.
// Must be called with t.mu held (read or write). Returns the session name and cache, or ("", nil).
func (t *Integration) findSessionCache(slug string, metadata map[string]string) (string, *sessionCache) {
	if name := metadata[session.MetaTmuxSession]; name != "" {
		if sc, exists := t.cache[name]; exists {
			return name, sc
		}
	}
	if sc, exists := t.cache[slug]; exists {
		return slug, sc
	}
	return "", nil
}

// DiscoverSession finds a tmux session for the given slug and metadata.
func (t *Integration) DiscoverSession(_ context.Context, slug string, metadata map[string]string) (*terminal.SessionInfo, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// Check if cache is fresh (2 second TTL)
	if t.cache == nil || time.Since(t.cacheTime) > 2*time.Second {
		return nil, nil
	}

	sessionName, sc := t.findSessionCache(slug, metadata)
	if sc == nil {
		return nil, nil
	}

	// If explicit window given, use it directly (check new key first, then legacy key).
	pane := metadata[session.MetaTmuxWindow]
	if pane == "" {
		pane = metadata["tmux_pane"] // migration fallback for legacy sessions
	}
	if pane != "" {
		w := sc.findWindow(pane)
		if w == nil {
			log.Debug().Str("session", sessionName).Str("pane", pane).Msg("explicit pane not found, falling back to best window")
			w = sc.bestWindow()
		}
		return t.sessionInfoFromWindow(sessionName, sc, w), nil
	}

	// Multi-window disambiguation
	w := t.disambiguateWindow(sc, metadata[sessionPathKey], slug)
	return t.sessionInfoFromWindow(sessionName, sc, w), nil
}

// disambiguateWindow selects the best window from a multi-window session cache.
// For single-window sessions, returns that window directly (backward compat).
// For multi-window sessions, disambiguates by:
//  1. Path match: window workDir matches sessionPath
//  2. Name match: window name contains the hive session slug (case-insensitive)
//  3. Fallback: highest-activity window
func (t *Integration) disambiguateWindow(sc *sessionCache, sessionPath, slug string) *agentWindow {
	if len(sc.agentWindows) <= 1 {
		return sc.bestWindow()
	}

	// 1. Path match
	if sessionPath != "" {
		for _, w := range sc.agentWindows {
			if w.workDir == sessionPath {
				return w
			}
		}
	}

	// 2. Name match (case-insensitive)
	if slug != "" {
		slugLower := strings.ToLower(slug)
		for _, w := range sc.agentWindows {
			if strings.Contains(strings.ToLower(w.windowName), slugLower) {
				return w
			}
		}
	}

	// 3. Fallback: highest-activity
	return sc.bestWindow()
}

// sessionInfoFromWindow builds a SessionInfo from a matched agentWindow.
// WindowName is always set so that template variables (e.g., .TmuxWindow) are
// populated regardless of whether the session has one or many windows.
func (t *Integration) sessionInfoFromWindow(sessionName string, _ *sessionCache, w *agentWindow) *terminal.SessionInfo {
	if w == nil {
		return &terminal.SessionInfo{Name: sessionName}
	}
	return &terminal.SessionInfo{
		Name:        sessionName,
		WindowIndex: w.windowIndex,
		PaneID:      w.primaryPaneID,
		WindowName:  w.windowName,
	}
}

// GetStatus returns the current status of a session.
func (t *Integration) GetStatus(ctx context.Context, info *terminal.SessionInfo) (terminal.Status, error) {
	if info == nil {
		return terminal.StatusMissing, nil
	}

	// Snapshot window state under lock to avoid races with RefreshCache and
	// concurrent GetStatus calls on the same window.
	t.mu.RLock()
	sc, exists := t.cache[info.Name]
	if !exists || len(sc.agentWindows) == 0 {
		t.mu.RUnlock()
		return terminal.StatusMissing, nil
	}

	w := sc.findWindow(info.WindowIndex)
	if w == nil {
		log.Debug().Str("session", info.Name).Str("window", info.WindowIndex).Msg("window not found in cache, falling back to best window")
		w = sc.bestWindow()
	}
	if w == nil {
		t.mu.RUnlock()
		return terminal.StatusMissing, nil
	}

	windowKey := info.Name + ":" + w.windowIndex
	prevContent := w.paneContent
	activity := w.activity
	lastCaptureActive := w.lastCaptureActive
	cachedStatus := w.cachedStatus
	windowIndex := w.windowIndex
	primaryPaneID := w.primaryPaneID
	sessionName := info.Name
	panePID := w.panePID
	t.mu.RUnlock()

	// Get or create rate limiter for this window
	t.mu.Lock()
	limiter, ok := t.limiters[windowKey]
	if !ok {
		limiter = terminal.NewRateLimiter(2) // Max 2 calls per second
		t.limiters[windowKey] = limiter
	}
	t.mu.Unlock()

	// Capture pane content only if activity changed and rate limit allows
	var content string
	switch {
	case prevContent != "" && activity == lastCaptureActive:
		// Activity hasn't changed, use cached content
		content = prevContent
	case !limiter.Allow():
		// Activity changed but rate limited, use cached content
		content = prevContent
	default:
		// Activity changed and rate limit allows, capture fresh.
		// Use pane ID directly if available (Phase 2+), else fall back to session:window.
		captureTarget := sessionName + ":" + windowIndex
		if primaryPaneID != "" {
			captureTarget = primaryPaneID
		}
		var err error
		content, err = t.capturePane(ctx, captureTarget)
		if err != nil {
			return terminal.StatusMissing, err
		}

		// Update cached content and activity timestamp.
		// Re-lookup the window under write lock in case RefreshCache swapped the cache.
		t.mu.Lock()
		if currentSC, ok := t.cache[sessionName]; ok {
			if currentW := currentSC.findWindow(windowIndex); currentW != nil {
				currentW.paneContent = content
				currentW.lastCaptureActive = activity
			}
		}
		t.mu.Unlock()
	}

	// Store pane content in SessionInfo for preview
	info.PaneContent = content

	// If content hasn't changed and we have a cached status, return it.
	// Compare against prevContent (the snapshot), not w.paneContent.
	if content == prevContent && cachedStatus != "" {
		return cachedStatus, nil
	}

	// Detect tool if not already set.
	// First try process-tree identification (more reliable than text patterns),
	// then fall back to text-pattern detection.
	tool := info.DetectedTool
	if tool == "" && panePID > 0 {
		if proc, procErr := process.Identify(int(panePID)); procErr == nil && proc != nil {
			tool = proc.Tool
			info.DetectedTool = tool
		}
	}
	if tool == "" {
		tool = terminal.DetectTool(content)
		info.DetectedTool = tool
	}

	// Get or create state tracker for this window
	t.mu.Lock()
	tracker, ok := t.trackers[windowKey]
	if !ok {
		tracker = terminal.NewStateTracker()
		t.trackers[windowKey] = tracker
	}
	t.mu.Unlock()

	// Use state tracker to determine status with spike detection
	detector := terminal.NewDetector(tool)
	status := tracker.Update(content, activity, detector)

	// Cache the status result.
	// Re-lookup the window under write lock in case RefreshCache swapped the cache.
	t.mu.Lock()
	if currentSC, ok := t.cache[sessionName]; ok {
		if currentW := currentSC.findWindow(windowIndex); currentW != nil {
			currentW.cachedStatus = status
		}
	}
	t.mu.Unlock()

	return status, nil
}

// capturePane captures the content of a tmux pane identified by target.
// target may be a pane ID (e.g. "%0") or a session:window address.
func (t *Integration) capturePane(_ context.Context, target string) (string, error) {
	// -p: print to stdout
	// -J: join wrapped lines and trim trailing spaces
	cmd := exec.Command("tmux", "capture-pane", "-t", target, "-p", "-J")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("capture-pane failed: %w", err)
	}

	return string(output), nil
}

// DiscoverAllWindows returns a SessionInfo for every tracked window in the tmux session
// matching the given slug and metadata. For single-window sessions it returns a single entry.
// This is used by the TUI to render each agent window as its own selectable tree item.
func (t *Integration) DiscoverAllWindows(_ context.Context, slug string, metadata map[string]string) ([]*terminal.SessionInfo, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.cache == nil || time.Since(t.cacheTime) > 2*time.Second {
		return nil, nil
	}

	sessionName, sc := t.findSessionCache(slug, metadata)

	if sc == nil || len(sc.agentWindows) == 0 {
		return nil, nil
	}

	infos := make([]*terminal.SessionInfo, 0, len(sc.agentWindows))
	for _, w := range sc.agentWindows {
		infos = append(infos, &terminal.SessionInfo{
			Name:        sessionName,
			WindowIndex: w.windowIndex,
			PaneID:      w.primaryPaneID,
			WindowName:  w.windowName,
		})
	}
	return infos, nil
}

// Ensure Integration implements terminal.Integration.
var _ terminal.Integration = (*Integration)(nil)
