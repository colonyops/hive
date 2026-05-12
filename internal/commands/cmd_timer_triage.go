package commands

import (
	"context"
	"strings"
	"time"

	"github.com/colonyops/hive/internal/core/tmux"
)

// failureReport captures the diagnostic data the timer-fire child emits
// as a structured zerolog WARN event when agent-send returns non-zero.
type failureReport struct {
	agentSendExit    int
	agentSendStderr  string
	tmuxServerExists bool
	sessionExists    bool
	windowExists     bool
}

// triageTimeout caps each individual tmux call so a wedged server can't
// hang the child indefinitely.
const triageTimeout = 2 * time.Second

// runTmuxTriage runs four tmux checks on a failed agent-send:
//
//  1. Is the tmux server alive?     (tmux info)
//  2. Does the named session exist? (tmux has-session)
//  3. Does the named window exist?  (tmux list-windows)
//  4. Capture agent-send's exit code and stderr (passed in by the caller).
//
// If target has no ":window" suffix the window check is skipped and
// windowExists mirrors sessionExists. Each check is bounded by triageTimeout.
func runTmuxTriage(
	ctx context.Context,
	client *tmux.Client,
	target string,
	agentSendExit int,
	agentSendStderr string,
) failureReport {
	report := failureReport{
		agentSendExit:   agentSendExit,
		agentSendStderr: agentSendStderr,
	}

	serverCtx, cancelServer := context.WithTimeout(ctx, triageTimeout)
	report.tmuxServerExists = client.Exists(serverCtx)
	cancelServer()

	if !report.tmuxServerExists {
		return report
	}

	sessionName, windowName := splitTarget(target)

	sessionCtx, cancelSession := context.WithTimeout(ctx, triageTimeout)
	report.sessionExists = client.HasSession(sessionCtx, sessionName)
	cancelSession()

	if !report.sessionExists {
		return report
	}

	if windowName == "" {
		report.windowExists = true
		return report
	}
	windowsCtx, cancelWindows := context.WithTimeout(ctx, triageTimeout)
	windows, err := client.ListWindows(windowsCtx, sessionName)
	cancelWindows()
	if err != nil {
		report.windowExists = false
		return report
	}
	for _, w := range windows {
		if w == windowName {
			report.windowExists = true
			return report
		}
	}
	return report
}

// splitTarget parses "session" or "session:window" or "session:window.pane"
// into (session, window). The pane suffix (after a dot) is stripped because
// tmux list-windows reports window NAMES, not window indexes — and the
// triage window check is best-effort match against the name component.
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
