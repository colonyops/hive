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

	"github.com/hay-kot/hive/internal/core/session"
	"github.com/hay-kot/hive/internal/integration/terminal"
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
		if re, err := regexp.Compile("(?i)" + p); err == nil { // case-insensitive
			patterns = append(patterns, re)
		}
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
		t.mu.Lock()
		t.cache = make(map[string]*sessionCache)
		t.cacheTime = time.Time{}
		t.mu.Unlock()
		return
	}

	t.mu.Lock()
	oldCache := t.cache
	t.mu.Unlock()

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

		// Preserve cached content from previous cache for this session:window
		if old, exists := oldCache[sessionName]; exists {
			for _, ow := range old.agentWindows {
				if ow.windowIndex == windowIndex {
					w.paneContent = ow.paneContent
					w.lastCaptureActive = ow.lastCaptureActive
					w.cachedStatus = ow.cachedStatus
					break
				}
			}
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

// DiscoverSession finds a tmux session for the given slug and metadata.
func (t *Integration) DiscoverSession(_ context.Context, slug string, metadata map[string]string) (*terminal.SessionInfo, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// Check if cache is fresh (2 second TTL)
	if t.cache == nil || time.Since(t.cacheTime) > 2*time.Second {
		return nil, nil
	}

	// First check metadata for explicit session name
	if sessionName := metadata[session.MetaTmuxSession]; sessionName != "" {
		if sc, exists := t.cache[sessionName]; exists {
			// If explicit pane given, use it directly
			if pane := metadata[session.MetaTmuxPane]; pane != "" {
				w := sc.findWindow(pane)
				if w == nil {
					w = sc.bestWindow()
				}
				return t.sessionInfoFromWindow(sessionName, sc, w), nil
			}
			// Multi-window disambiguation
			w := t.disambiguateWindow(sc, metadata[sessionPathKey], slug)
			return t.sessionInfoFromWindow(sessionName, sc, w), nil
		}
	}

	// Try exact slug match
	if sc, exists := t.cache[slug]; exists {
		w := t.disambiguateWindow(sc, metadata[sessionPathKey], slug)
		return t.sessionInfoFromWindow(slug, sc, w), nil
	}

	// Try prefix match (session name starts with slug)
	for name, sc := range t.cache {
		if strings.HasPrefix(name, slug+"_") || strings.HasPrefix(name, slug+"-") {
			w := t.disambiguateWindow(sc, metadata[sessionPathKey], slug)
			return t.sessionInfoFromWindow(name, sc, w), nil
		}
	}

	return nil, nil
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
		Name:       sessionName,
		Pane:       w.windowIndex,
		WindowName: w.windowName,
	}
}

// GetStatus returns the current status of a session.
func (t *Integration) GetStatus(ctx context.Context, info *terminal.SessionInfo) (terminal.Status, error) {
	if info == nil {
		return terminal.StatusMissing, nil
	}

	// Check session exists and find the specific window
	t.mu.RLock()
	sc, exists := t.cache[info.Name]
	t.mu.RUnlock()

	if !exists || len(sc.agentWindows) == 0 {
		return terminal.StatusMissing, nil
	}

	// Find the specific window by Pane (window index)
	w := sc.findWindow(info.Pane)
	if w == nil {
		w = sc.bestWindow()
	}
	if w == nil {
		return terminal.StatusMissing, nil
	}

	// Key for per-window rate limiters and trackers
	windowKey := info.Name + ":" + w.windowIndex

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
	case w.paneContent != "" && w.activity == w.lastCaptureActive:
		// Activity hasn't changed, use cached content
		content = w.paneContent
	case !limiter.Allow():
		// Activity changed but rate limited, use cached content
		content = w.paneContent
	default:
		// Activity changed and rate limit allows, capture fresh
		var err error
		content, err = t.capturePane(ctx, info.Name, w.windowIndex)
		if err != nil {
			return terminal.StatusMissing, err
		}

		// Update cached content and activity timestamp
		t.mu.Lock()
		w.paneContent = content
		w.lastCaptureActive = w.activity
		t.mu.Unlock()
	}

	// Store pane content in SessionInfo for preview
	info.PaneContent = content

	// If content hasn't changed and we have a cached status, return it
	// This avoids expensive detector string operations on unchanged content
	if content == w.paneContent && w.cachedStatus != "" {
		return w.cachedStatus, nil
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
	status := tracker.Update(content, w.activity, detector)

	// Cache the status result for future polls
	t.mu.Lock()
	w.cachedStatus = status
	t.mu.Unlock()

	return status, nil
}

// capturePane captures the content of a tmux pane.
func (t *Integration) capturePane(_ context.Context, sessionName, pane string) (string, error) {
	target := sessionName
	if pane != "" {
		target = sessionName + ":" + pane
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

	// Find the session cache using the same lookup logic as DiscoverSession.
	var sessionName string
	var sc *sessionCache

	if name := metadata[session.MetaTmuxSession]; name != "" {
		if c, exists := t.cache[name]; exists {
			sessionName = name
			sc = c
		}
	}
	if sc == nil {
		if c, exists := t.cache[slug]; exists {
			sessionName = slug
			sc = c
		}
	}
	if sc == nil {
		for name, c := range t.cache {
			if strings.HasPrefix(name, slug+"_") || strings.HasPrefix(name, slug+"-") {
				sessionName = name
				sc = c
				break
			}
		}
	}

	if sc == nil || len(sc.agentWindows) == 0 {
		return nil, nil
	}

	infos := make([]*terminal.SessionInfo, 0, len(sc.agentWindows))
	for _, w := range sc.agentWindows {
		infos = append(infos, &terminal.SessionInfo{
			Name:       sessionName,
			Pane:       w.windowIndex,
			WindowName: w.windowName,
		})
	}
	return infos, nil
}

// Ensure Integration implements terminal.Integration.
var _ terminal.Integration = (*Integration)(nil)
