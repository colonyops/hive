package terminal

import (
	"context"
	"os"
	"os/exec"
	"strings"

	"github.com/colonyops/hive/internal/core/messaging"
	"github.com/colonyops/hive/internal/core/session"
)

// HookResolver identifies the Hive session and tmux window index for a Claude
// Code hook event. It uses a waterfall of three strategies, cheapest first:
//
//  1. HIVE_SESSION_ID env var (injected by hive at spawn time — Phase 3)
//  2. tmux session name: matches #{session_name} against sess.Slug
//  3. CWD path match: delegates to messaging.DetectSessionFromPath
type HookResolver struct {
	store session.Store
}

// NewHookResolver creates a new HookResolver backed by the given session store.
func NewHookResolver(store session.Store) *HookResolver {
	return &HookResolver{store: store}
}

// Resolve returns the Hive session ID and tmux window index for the current
// hook invocation. sessionID is empty if no matching session is found — the
// caller should exit 0 silently (hook fired outside a Hive session).
// windowIndex defaults to "0" when not running inside tmux.
func (r *HookResolver) Resolve(ctx context.Context) (sessionID, windowIndex string, err error) {
	windowIndex = r.windowIndex()

	// Strategy 1: HIVE_SESSION_ID env var (fastest, set in Phase 3)
	if id := os.Getenv("HIVE_SESSION_ID"); id != "" {
		return id, windowIndex, nil
	}

	// Strategy 2: tmux session name matches sess.Slug
	if tmuxName := tmuxSessionName(); tmuxName != "" {
		sessions, err := r.store.List(ctx)
		if err != nil {
			return "", windowIndex, err
		}
		for _, sess := range sessions {
			if sess.State == session.StateActive && sess.Slug == tmuxName {
				return sess.ID, windowIndex, nil
			}
		}
	}

	// Strategy 3: CWD path match
	detector := messaging.NewSessionDetector(r.store)
	id, err := detector.DetectSession(ctx)
	return id, windowIndex, err
}

// windowIndex returns the tmux window index for the current process, or "0"
// if not running inside tmux.
func (r *HookResolver) windowIndex() string {
	if os.Getenv("TMUX") == "" {
		return "0"
	}
	out, err := exec.Command("tmux", "display-message", "-p", "#{window_index}").Output()
	if err != nil {
		return "0"
	}
	idx := strings.TrimSpace(string(out))
	if idx == "" {
		return "0"
	}
	return idx
}

// tmuxSessionName returns the current tmux session name, or empty string if
// not inside tmux.
func tmuxSessionName() string {
	if os.Getenv("TMUX") == "" {
		return ""
	}
	out, err := exec.Command("tmux", "display-message", "-p", "#{session_name}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// MapEventToStatus maps a Claude Code hook event name to a Hive terminal Status.
// Returns (status, true) for known events, ("", false) for unknown events.
func MapEventToStatus(event string) (Status, bool) {
	switch event {
	case "SessionStart", "Stop":
		return StatusReady, true
	case "UserPromptSubmit":
		return StatusActive, true
	case "PermissionRequest", "Notification":
		return StatusApproval, true
	case "SessionEnd":
		return StatusMissing, true
	default:
		return "", false
	}
}
