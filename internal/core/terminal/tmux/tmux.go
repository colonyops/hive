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
	windowIndex       string // window index for targeted capture (e.g., "0", "1")
	windowName        string // window name for display and matching
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
	// Get session name, window index, window name, work dir, and activity
	cmd := exec.Command("tmux", "list-windows", "-a", "-F",
		"#{session_name}\t#{window_index}\t#{window_name}\t#{pane_current_path}\t#{window_activity}")
	output, err := cmd.Output()
	if err != nil {
		log.Debug().Err(err).Msg("tmux list-windows failed, clearing cache")
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

	// First pass: collect all windows grouped by tmux session, tracking preferred status
	type windowEntry struct {
		window    *agentWindow
		preferred bool
	}
	collected := make(map[string][]windowEntry)

	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 5)
		if len(parts) < 3 {
			continue
		}

		sessionName := parts[0]
		windowIndex := parts[1]
		windowName := parts[2]

		w := &agentWindow{
			windowIndex: windowIndex,
			windowName:  windowName,
		}

		if len(parts) >= 4 {
			w.workDir = parts[3]
		}
		if len(parts) >= 5 {
			_, _ = fmt.Sscanf(parts[4], "%d", &w.activity)
		}

		// Preserve cached content from snapshot
		if snap, exists := oldWindows[sessionName+":"+windowIndex]; exists {
			w.paneContent = snap.paneContent
			w.lastCaptureActive = snap.lastCaptureActive
			w.cachedStatus = snap.cachedStatus
		}

		isPreferred := t.matchesPreferredWindow(windowName)
		collected[sessionName] = append(collected[sessionName], windowEntry{
			window:    w,
			preferred: isPreferred,
		})
	}

	// Second pass: for each tmux session, decide which windows to keep.
	// If any windows match preferred patterns, keep all preferred windows.
	// Otherwise keep only the single highest-activity window (backward compat).
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
			// No preferred windows: keep the single highest-activity window
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

// findSessionCache locates the sessionCache for a slug using metadata, exact match, or prefix match.
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
	for name, sc := range t.cache {
		if strings.HasPrefix(name, slug+"_") || strings.HasPrefix(name, slug+"-") {
			return name, sc
		}
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

	// If explicit window index given, use it directly
	if windowIndex := metadata[session.MetaTmuxWindow]; windowIndex != "" {
		w := sc.findWindow(windowIndex)
		if w == nil {
			log.Debug().Str("session", sessionName).Str("window", windowIndex).Msg("explicit window not found, falling back to best window")
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
	sessionName := info.Name
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
		// Activity changed and rate limit allows, capture fresh
		var err error
		content, err = t.captureWindow(ctx, info.Name, windowIndex)
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

	// Detect tool if not already set
	tool := info.DetectedTool
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

// captureWindow captures the content of a tmux window.
// Always targets pane 0: hive creates agent commands via new-window which places
// the process in pane 0. Additional panes (companion tools) are appended after,
// so pane 0 is always the agent pane.
// Assumes pane-base-index=0 (tmux default); non-default configurations will target
// the wrong pane and may cause status detection to return StatusMissing.
func (t *Integration) captureWindow(_ context.Context, sessionName, windowIndex string) (string, error) {
	target := sessionName
	if windowIndex != "" {
		target = sessionName + ":" + windowIndex + ".0"
	}

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
			WindowName:  w.windowName,
		})
	}
	return infos, nil
}

// Ensure Integration implements terminal.Integration.
var _ terminal.Integration = (*Integration)(nil)
