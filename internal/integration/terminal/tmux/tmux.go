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
	cache            map[string]sessionCache
	cacheTime        time.Time
	trackers         map[string]*terminal.StateTracker
	limiters         map[string]*terminal.RateLimiter
	available        bool
	availableOnce    sync.Once
	preferredWindows []*regexp.Regexp // compiled patterns for preferred window names
}

type sessionCache struct {
	workDir           string
	windowIndex       string // window index for targeted capture (e.g., "0", "1")
	windowName        string // window name for debugging
	activity          int64
	paneContent       string
	lastCaptureActive int64
	cachedStatus      terminal.Status
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
		cache:            make(map[string]sessionCache),
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
		t.cache = make(map[string]sessionCache)
		t.cacheTime = time.Time{}
		t.mu.Unlock()
		return
	}

	t.mu.Lock()
	oldCache := t.cache
	t.mu.Unlock()

	newCache := make(map[string]sessionCache)
	// Track which sessions have a preferred window match
	preferredMatch := make(map[string]bool)

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

		entry := sessionCache{
			windowIndex: windowIndex,
			windowName:  windowName,
		}

		// Preserve cached content from previous cache for this session
		if old, exists := oldCache[sessionName]; exists {
			entry.paneContent = old.paneContent
			entry.lastCaptureActive = old.lastCaptureActive
			entry.cachedStatus = old.cachedStatus
		}

		if len(parts) >= 4 {
			entry.workDir = parts[3]
		}
		if len(parts) >= 5 {
			_, _ = fmt.Sscanf(parts[4], "%d", &entry.activity)
		}

		// Check if this window matches any preferred pattern
		isPreferred := t.matchesPreferredWindow(windowName)

		// Decide whether to use this window for the session
		existing, hasExisting := newCache[sessionName]
		existingIsPreferred := preferredMatch[sessionName]

		shouldReplace := false
		switch {
		case !hasExisting:
			// No existing entry, use this one
			shouldReplace = true
		case isPreferred && !existingIsPreferred:
			// This is preferred, existing is not - prefer this one
			shouldReplace = true
		case isPreferred == existingIsPreferred && entry.activity > existing.activity:
			// Same preference level, use more recent activity
			shouldReplace = true
		}

		if shouldReplace {
			newCache[sessionName] = entry
			preferredMatch[sessionName] = isPreferred
		}
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
		if _, exists := t.cache[sessionName]; exists {
			return &terminal.SessionInfo{
				Name: sessionName,
				Pane: metadata[session.MetaTmuxPane],
			}, nil
		}
	}

	// Try exact slug match
	if _, exists := t.cache[slug]; exists {
		return &terminal.SessionInfo{
			Name: slug,
		}, nil
	}

	// Try prefix match (session name starts with slug)
	for name := range t.cache {
		if strings.HasPrefix(name, slug+"_") || strings.HasPrefix(name, slug+"-") {
			return &terminal.SessionInfo{
				Name: name,
			}, nil
		}
	}

	return nil, nil
}

// GetStatus returns the current status of a session.
func (t *Integration) GetStatus(ctx context.Context, info *terminal.SessionInfo) (terminal.Status, error) {
	if info == nil {
		return terminal.StatusMissing, nil
	}

	// Check session exists and get activity info
	t.mu.RLock()
	cached, exists := t.cache[info.Name]
	t.mu.RUnlock()

	if !exists {
		return terminal.StatusMissing, nil
	}

	// Get or create rate limiter for this session
	t.mu.Lock()
	limiter, ok := t.limiters[info.Name]
	if !ok {
		limiter = terminal.NewRateLimiter(2) // Max 2 calls per second
		t.limiters[info.Name] = limiter
	}
	t.mu.Unlock()

	// Capture pane content only if activity changed and rate limit allows
	var content string
	switch {
	case cached.paneContent != "" && cached.activity == cached.lastCaptureActive:
		// Activity hasn't changed, use cached content
		content = cached.paneContent
	case !limiter.Allow():
		// Activity changed but rate limited, use cached content
		content = cached.paneContent
	default:
		// Activity changed and rate limit allows, capture fresh
		// Use cached window index to target the preferred window
		var err error
		content, err = t.capturePane(ctx, info.Name, cached.windowIndex)
		if err != nil {
			return terminal.StatusMissing, err
		}

		// Update cache with new content and activity timestamp
		t.mu.Lock()
		if entry, ok := t.cache[info.Name]; ok {
			entry.paneContent = content
			entry.lastCaptureActive = cached.activity
			t.cache[info.Name] = entry
		}
		t.mu.Unlock()
	}

	// Store pane content in SessionInfo for preview
	info.PaneContent = content

	// If content hasn't changed and we have a cached status, return it
	// This avoids expensive detector string operations on unchanged content
	if content == cached.paneContent && cached.cachedStatus != "" {
		return cached.cachedStatus, nil
	}

	// Detect tool if not already set
	tool := info.DetectedTool
	if tool == "" {
		tool = terminal.DetectTool(content)
		info.DetectedTool = tool
	}

	// Get or create state tracker for this session
	t.mu.Lock()
	tracker, ok := t.trackers[info.Name]
	if !ok {
		tracker = terminal.NewStateTracker()
		t.trackers[info.Name] = tracker
	}
	t.mu.Unlock()

	// Use state tracker to determine status with spike detection
	detector := terminal.NewDetector(tool)
	status := tracker.Update(content, cached.activity, detector)

	// Cache the status result for future polls
	t.mu.Lock()
	if entry, ok := t.cache[info.Name]; ok {
		entry.cachedStatus = status
		t.cache[info.Name] = entry
	}
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

// Ensure Integration implements terminal.Integration.
var _ terminal.Integration = (*Integration)(nil)
