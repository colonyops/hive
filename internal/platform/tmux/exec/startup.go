package tmuxexec

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// ErrCommandExited matches a CommandExitedError with errors.Is.
var ErrCommandExited = errors.New("tmux window command exited during startup")

const (
	exitStatusNotFound  = 127
	defaultStartupGrace = 250 * time.Millisecond
	startupPollInterval = 50 * time.Millisecond
	maxStartupOutput    = 32 * 1024
)

// CommandExitedError reports a failed or lost pane during session startup.
// Output is a best-effort terminal excerpt, not a separate stderr stream.
// Its message leaves out the session name because a LaunchError, which names
// the session, always wraps it.
type CommandExitedError struct {
	Session      string
	Window       string
	Pane         string
	Command      string
	Status       int
	Signal       string
	Lost         bool
	Output       string
	Truncated    bool
	CaptureError error
}

func (e *CommandExitedError) Error() string {
	var message string
	switch {
	case e.Lost:
		message = fmt.Sprintf("pane %s disappeared in window %q during startup. No exit details are available.", e.Pane, e.Window)
	case e.Signal != "":
		message = fmt.Sprintf("command terminated in window %q during startup (signal %s)", e.Window, e.Signal)
	case e.NotFound():
		message = fmt.Sprintf("command not found in window %q (status 127)", e.Window)
	case e.Status >= 0:
		message = fmt.Sprintf("command exited in window %q during startup (status %d)", e.Window, e.Status)
	default:
		message = fmt.Sprintf("command terminated in window %q during startup. No exit details are available.", e.Window)
	}
	if e.Command != "" {
		message += "\n\n$ " + e.Command
	}
	if e.Output != "" {
		message += "\n" + e.Output
	}
	if e.Truncated {
		message += "\n[terminal output truncated]"
	}
	if e.CaptureError != nil {
		message += "\nTerminal output unavailable: " + e.CaptureError.Error()
	}
	return message
}

func (e *CommandExitedError) Is(target error) bool { return target == ErrCommandExited }

// NotFound reports the shell's command-not-found exit status.
func (e *CommandExitedError) NotFound() bool { return e.Status == exitStatusNotFound && e.Signal == "" }

const startupPaneFormat = "#{pane_id}|#{pane_dead}|#{pane_dead_status}|#{pane_dead_signal}"

type paneObservation struct {
	dead   bool
	status int
	signal string
}

func (l *launch) observe(ctx context.Context) error {
	watching := false
	for _, window := range l.windows {
		for _, pane := range window.panes {
			watching = watching || pane.state != PaneLaunchStateCompleted
		}
	}
	if !watching {
		return nil
	}
	for {
		observations, err := l.inspect(ctx)
		if err != nil {
			return err
		}
		var failures []error
		var remaining time.Duration
		for _, window := range l.windows {
			for _, pane := range window.panes {
				if pane.state == PaneLaunchStateCompleted {
					continue
				}
				observation, exists := observations[pane.id]
				if !exists {
					pane.state = PaneLaunchStateLost
					failures = append(failures, l.paneFailure(ctx, pane, paneObservation{status: -1}))
					continue
				}
				if observation.dead {
					if observation.status == 0 && observation.signal == "" {
						pane.state = PaneLaunchStateCompleted
						continue
					}
					if observation.status < 0 && observation.signal == "" && time.Until(pane.deadline) > 0 {
						pane.state = PaneLaunchStateExitPending
					} else {
						pane.state = PaneLaunchStateFailed
						failures = append(failures, l.paneFailure(ctx, pane, observation))
						continue
					}
				} else if time.Until(pane.deadline) <= 0 {
					pane.state = PaneLaunchStateRunning
				}
				remaining = max(remaining, time.Until(pane.deadline))
			}
		}
		if len(failures) > 0 {
			return errors.Join(failures...)
		}
		if remaining <= 0 {
			return nil
		}
		timer := time.NewTimer(min(remaining, startupPollInterval))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (l *launch) inspect(ctx context.Context) (map[string]paneObservation, error) {
	stdout, _, err := l.client.runner.Capture(ctx, "list-panes", "-s", "-t", l.sessionID, "-F", startupPaneFormat)
	if err != nil {
		return nil, fmt.Errorf("tmux list-panes during startup: %w", err)
	}
	observations := make(map[string]paneObservation)
	for line := range strings.Lines(string(stdout)) {
		fields := strings.Split(strings.TrimSuffix(line, "\n"), "|")
		if len(fields) != 4 || !validNativeID(fields[0], '%') || (fields[1] != "0" && fields[1] != "1") {
			return nil, fmt.Errorf("invalid tmux startup pane metadata %q", line)
		}
		observation := paneObservation{dead: fields[1] == "1", status: -1, signal: fields[3]}
		if fields[2] != "" {
			status, parseErr := strconv.Atoi(fields[2])
			if parseErr != nil || status < 0 {
				return nil, fmt.Errorf("invalid tmux pane exit status %q", fields[2])
			}
			observation.status = status
		}
		observations[fields[0]] = observation
	}
	return observations, nil
}

func (l *launch) paneFailure(ctx context.Context, pane *launchPane, observation paneObservation) *CommandExitedError {
	err := &CommandExitedError{Session: l.name, Window: pane.window, Pane: pane.id, Command: pane.command, Status: observation.status, Signal: observation.signal, Lost: pane.state == PaneLaunchStateLost}
	if err.Lost {
		return err
	}
	stdout, _, captureErr := l.client.runner.Capture(ctx, "capture-pane", "-p", "-J", "-t", pane.id, "-S", "-20")
	if captureErr != nil {
		l.client.log.Debug().Err(captureErr).Str("pane", pane.id).Msg("failed to capture terminated pane output")
		err.CaptureError = captureErr
		return err
	}
	err.Output = strings.TrimRight(string(stdout), "\r\n ")
	if len(err.Output) > maxStartupOutput {
		err.Truncated = true
		err.Output = err.Output[len(err.Output)-maxStartupOutput:]
		for len(err.Output) > 0 && !utf8.RuneStart(err.Output[0]) {
			err.Output = err.Output[1:]
		}
	}
	return err
}
