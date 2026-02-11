package terminal

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"time"
)

// StateTracker tracks terminal activity state across poll cycles.
// Implements spike detection to filter cursor blinks and terminal redraws.
//
// Three-state model:
//   - GREEN (active)   = Explicit busy indicator found (spinner, "ctrl+c to interrupt")
//   - YELLOW (approval) = Permission dialog detected, needs user decision
//   - CYAN (ready)     = Input prompt detected, ready for next task
type StateTracker struct {
	// Content tracking
	lastHash       string    // SHA256 of normalized content
	lastChangeTime time.Time // When sustained activity was last confirmed

	// Activity timestamp tracking (from tmux window_activity)
	lastActivityTimestamp int64 // Previous activity timestamp

	// Spike detection: track activity changes across poll cycles
	// Requires 2+ timestamp changes within 1 second to confirm sustained activity
	activityCheckStart  time.Time // When we started tracking for sustained activity
	activityChangeCount int       // How many timestamp changes seen in current window

	// Last stable status (returned during spike detection window)
	lastStableStatus Status

	// Hysteresis: minimum time to hold a status before allowing change
	lastStatusTime time.Time // When current status was set
}

// SpikeWindow is how long we wait to confirm sustained activity.
const SpikeWindow = 1 * time.Second

// HysteresisWindow is the minimum time to hold a status before changing.
// This prevents rapid flickering between states from status bar updates.
const HysteresisWindow = 500 * time.Millisecond

// NewStateTracker creates a new state tracker.
func NewStateTracker() *StateTracker {
	return &StateTracker{
		lastStableStatus: StatusReady,
	}
}

// Update processes new activity data and returns the detected status.
// content is the terminal content (for busy/prompt detection).
// activityTS is the tmux window_activity timestamp.
// detector is used to check busy/approval/ready patterns.
func (st *StateTracker) Update(content string, activityTS int64, detector *Detector) Status {
	now := time.Now()

	// Check for explicit indicators (most reliable)
	isBusy := detector.IsBusy(content)
	needsApproval := detector.NeedsApproval(content)
	isReady := detector.IsReady(content)

	// Determine what status we would like to transition to
	var desiredStatus Status
	switch {
	case needsApproval:
		// Approval takes highest priority (Claude is blocked)
		desiredStatus = StatusApproval
	case isBusy:
		// Busy indicator = definitely active
		desiredStatus = StatusActive
	case isReady:
		// Ready (prompt visible)
		desiredStatus = StatusReady
	default:
		// No explicit indicators - stay at current status or default to ready
		desiredStatus = st.lastStableStatus
		if desiredStatus == "" {
			desiredStatus = StatusReady
		}
	}

	// Apply hysteresis: don't change status too quickly
	// Exception: approval status always gets through immediately (user is waiting)
	if desiredStatus != st.lastStableStatus {
		if desiredStatus != StatusApproval && !st.lastStatusTime.IsZero() {
			if now.Sub(st.lastStatusTime) < HysteresisWindow {
				// Within hysteresis window, keep current status
				return st.lastStableStatus
			}
		}

		// Transition to new status
		st.lastStableStatus = desiredStatus
		st.lastStatusTime = now
		st.resetSpikeDetection()

		if desiredStatus == StatusActive {
			st.lastChangeTime = now
		}

		return desiredStatus
	}

	// Same status - update timestamp tracking for activity monitoring
	if st.lastActivityTimestamp == 0 {
		st.lastActivityTimestamp = activityTS
	} else if st.lastActivityTimestamp != activityTS {
		st.lastActivityTimestamp = activityTS
		// Activity changed but status didn't - this is normal (status bar updates)
		// Reset spike detection to avoid accumulating false positives
		st.resetSpikeDetection()
	}

	return st.lastStableStatus
}

// resetSpikeDetection clears the spike detection window.
func (st *StateTracker) resetSpikeDetection() {
	st.activityCheckStart = time.Time{}
	st.activityChangeCount = 0
}

// UpdateHash updates the content hash and returns true if content changed.
func (st *StateTracker) UpdateHash(content string) bool {
	normalized := NormalizeContent(content)
	hash := HashContent(normalized)
	if hash == st.lastHash {
		return false
	}
	st.lastHash = hash
	return true
}

// spinnerRunes are characters stripped during content normalization.
var spinnerRunes = []rune{
	'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏', // braille
	'·', '✳', '✽', '✶', '✻', '✢', // asterisk spinners
}

// Patterns for normalizing dynamic content.
var (
	// Dynamic status counters: "(45s · 1234 tokens · ctrl+c to interrupt)" or "(35s · ↑ 673 tokens)"
	dynamicStatusPattern = regexp.MustCompile(`\([^)]*\d+s\s*·[^)]*(?:tokens|↑|↓)[^)]*\)`)

	// Progress bar patterns: [====>   ] 45%
	progressBarPattern = regexp.MustCompile(`\[=*>?\s*\]\s*\d+%`)

	// Time patterns like 12:34 or 12:34:56
	timePattern = regexp.MustCompile(`\b\d{1,2}:\d{2}(:\d{2})?\b`)

	// Progress percentages like 45%
	percentagePattern = regexp.MustCompile(`\b\d{1,3}%`)

	// Download progress like 1.2MB/5.6MB
	downloadPattern = regexp.MustCompile(`\d+(\.\d+)?[KMGT]?B/\d+(\.\d+)?[KMGT]?B`)

	// Multiple blank lines
	blankLinesPattern = regexp.MustCompile(`\n{3,}`)

	// Thinking pattern with spinner + ellipsis + status: "✳ Gusting… (35s · ↑ 673 tokens)"
	thinkingPatternEllipsis = regexp.MustCompile(`[⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏·✳✽✶✻✢]\s*.+…\s*\([^)]*\)`)

	// Status line patterns (Claude Code status bar)
	// Example: "📂 hive-fsnotify-co4xf6 • 🌿 feat/fsnotify-watcher • 🗃 50/57"
	statusLinePattern = regexp.MustCompile(`📂[^•]+•[^•]+•[^•\n]+`)

	// Git branch in status: "🌿 branch-name" or "🌿 main"
	gitBranchStatusPattern = regexp.MustCompile(`🌿\s*[a-zA-Z0-9/_-]+`)

	// Beads count: "🗃 50/57" or similar
	beadsCountPattern = regexp.MustCompile(`🗃\s*\d+/\d+`)
)

// NormalizeContent prepares content for hashing by removing dynamic elements.
// This prevents false hash changes from animations and counters.
func NormalizeContent(content string) string {
	result := StripANSI(content)

	// Strip control characters (keep tab, newline, carriage return)
	result = stripControlChars(result)

	// Strip spinner characters that animate
	for _, r := range spinnerRunes {
		result = strings.ReplaceAll(result, string(r), "")
	}

	// Normalize Claude Code dynamic status: "(45s · 1234 tokens)" → "(STATUS)"
	result = dynamicStatusPattern.ReplaceAllString(result, "(STATUS)")

	// Normalize thinking spinner patterns: "✳ Gusting… (35s · ↑ 673 tokens)" → "THINKING…"
	result = thinkingPatternEllipsis.ReplaceAllString(result, "THINKING…")

	// Normalize progress indicators
	result = progressBarPattern.ReplaceAllString(result, "[PROGRESS]")
	result = downloadPattern.ReplaceAllString(result, "X.XMB/Y.YMB")
	result = percentagePattern.ReplaceAllString(result, "N%")

	// Normalize time patterns that change every second
	result = timePattern.ReplaceAllString(result, "HH:MM:SS")

	// Normalize status line patterns (Claude Code status bar that updates frequently)
	result = statusLinePattern.ReplaceAllString(result, "[STATUSLINE]")
	result = gitBranchStatusPattern.ReplaceAllString(result, "[BRANCH]")
	result = beadsCountPattern.ReplaceAllString(result, "[BEADS]")

	// Trim trailing whitespace per line (fixes resize false positives)
	lines := strings.Split(result, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	result = strings.Join(lines, "\n")

	// Collapse multiple blank lines
	result = blankLinesPattern.ReplaceAllString(result, "\n\n")

	return result
}

// stripControlChars removes ASCII control characters except tab, newline, CR.
func stripControlChars(content string) string {
	var result strings.Builder
	result.Grow(len(content))
	for _, r := range content {
		if (r >= 32 && r != 127) || r == '\t' || r == '\n' || r == '\r' {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// HashContent generates SHA256 hash of content.
func HashContent(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}
