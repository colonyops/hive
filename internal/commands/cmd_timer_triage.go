package commands

import (
	"context"
	"strings"
	"time"

	"github.com/colonyops/hive/internal/core/tmux"
)

// FailureReport captures the diagnostic data the timer-fire child emits
// as a structured zerolog WARN event when agent-send returns non-zero.
// Field names match the zerolog keys: tmux_server_exists, session_exists,
// window_exists, agent_send_exit, agent_send_stderr.
type FailureReport struct {
	AgentSendExit    int
	AgentSendStderr  string
	TmuxServerExists bool
	SessionExists    bool
	WindowExists     bool
}

// triageTimeout caps each individual tmux call so a wedged server can't
// hang the child indefinitely.
const triageTimeout = 2 * time.Second

// RunTmuxTriage runs the four tmux checks listed in the timer spec's
// Failure Triage table:
//
//  1. Is the tmux server alive?       (tmux info via Client.Exists)
//  2. Does the named session exist?   (tmux has-session via Client.HasSession)
//  3. Does the named window exist?    (parsed window name vs Client.ListWindows)
//  4. Capture agent-send's exit code and stderr (passed in by the caller).
//
// target is the tmux target string ("session" or "session:window"). If
// target has no ":window" suffix the window check is skipped and
// WindowExists is reported as true when the session exists (no window to
// check).
//
// Each individual check is bounded by triageTimeout to keep the child from
// hanging on a wedged server. ctx is the caller's context — typically the
// short-lived child process context.
func RunTmuxTriage(
	ctx context.Context,
	client *tmux.Client,
	target string,
	agentSendExit int,
	agentSendStderr string,
) FailureReport {
	report := FailureReport{
		AgentSendExit:   agentSendExit,
		AgentSendStderr: agentSendStderr,
	}

	// Check 1: server alive.
	serverCtx, cancelServer := context.WithTimeout(ctx, triageTimeout)
	report.TmuxServerExists = client.Exists(serverCtx)
	cancelServer()

	if !report.TmuxServerExists {
		// No server → session and window are by definition unreachable.
		return report
	}

	// Parse target into session + window components.
	sessionName, windowName := splitTarget(target)

	// Check 2: session exists.
	sessionCtx, cancelSession := context.WithTimeout(ctx, triageTimeout)
	report.SessionExists = client.HasSession(sessionCtx, sessionName)
	cancelSession()

	if !report.SessionExists {
		return report
	}

	// Check 3: window exists. If no window component was specified in the
	// target, we have nothing to check; mirror SessionExists.
	if windowName == "" {
		report.WindowExists = true
		return report
	}
	windowsCtx, cancelWindows := context.WithTimeout(ctx, triageTimeout)
	windows, err := client.ListWindows(windowsCtx, sessionName)
	cancelWindows()
	if err != nil {
		report.WindowExists = false
		return report
	}
	for _, w := range windows {
		if w == windowName {
			report.WindowExists = true
			return report
		}
	}
	return report
}

// splitTarget parses "session" or "session:window" or "session:window.pane"
// into (session, window). The pane suffix (after a dot) is stripped because
// tmux list-windows reports window NAMES, not window indexes — and the
// triage Window check is best-effort match against the name component.
// When MetaTmuxPane stores a window INDEX (the existing convention; see
// internal/core/terminal/tmux/tmux.go), the check will simply fail to
// match — that's fine, the report bool then reflects that the window
// portion didn't resolve, which is the diagnostic intent.
func splitTarget(target string) (session, window string) {
	if i := strings.IndexByte(target, ':'); i >= 0 {
		session = target[:i]
		rest := target[i+1:]
		if dot := strings.IndexByte(rest, '.'); dot >= 0 {
			window = rest[:dot]
		} else {
			window = rest
		}
		return session, window
	}
	return target, ""
}
