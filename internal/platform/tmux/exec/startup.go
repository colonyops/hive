package tmuxexec

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ErrCommandExited matches a CommandExitedError with errors.Is.
var ErrCommandExited = errors.New("tmux window command exited during startup")

const (
	exitStatusNotFound  = 127
	defaultStartupGrace = 250 * time.Millisecond
	startupPollInterval = 50 * time.Millisecond
)

// CommandExitedError reports a window command that exited while Hive was
// creating its tmux session.
type CommandExitedError struct {
	Session string
	Window  string
	Command string
	Status  int
	Output  string
}

func (e *CommandExitedError) Error() string {
	var message string
	switch {
	case e.NotFound():
		message = fmt.Sprintf("tmux session %q: command not found in window %q (status 127)", e.Session, e.Window)
	case e.Status >= 0:
		message = fmt.Sprintf("tmux session %q: command exited in window %q during startup (status %d)", e.Session, e.Window, e.Status)
	default:
		message = fmt.Sprintf("tmux session %q: command exited in window %q during startup", e.Session, e.Window)
	}
	if e.Command != "" {
		message += "\n\n$ " + e.Command
	}
	if e.Output != "" {
		message += "\n" + e.Output
	}
	return message
}

func (e *CommandExitedError) Is(target error) bool {
	return target == ErrCommandExited
}

// NotFound reports whether the shell could not find the command.
func (e *CommandExitedError) NotFound() bool {
	return e.Status == exitStatusNotFound
}

type startedPane struct {
	id      string
	window  string
	command string
}

func watchPane(id, window, command string) []startedPane {
	if id == "" {
		return nil
	}
	return []startedPane{{id: id, window: window, command: command}}
}

func (c *Client) awaitStartup(ctx context.Context, sessionName string, started []startedPane) error {
	if len(started) == 0 {
		return nil
	}

	deadline := time.Now().Add(c.startupGrace)
	for {
		exited, err := c.findExitedPane(ctx, sessionName, started)
		if err != nil {
			return err
		}
		if exited != nil {
			return exited
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		timer := time.NewTimer(min(remaining, startupPollInterval))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}

	cleared := make(map[string]struct{}, len(started))
	for _, pane := range started {
		if _, ok := cleared[pane.window]; ok {
			continue
		}
		cleared[pane.window] = struct{}{}
		if _, _, err := c.runner.Capture(ctx, "set-option", "-w", "-u", "-t", pane.id, "remain-on-exit"); err != nil {
			c.log.Warn().Err(err).Str("pane", pane.id).Msg("failed to clear remain-on-exit")
		}
	}
	return nil
}

func (c *Client) findExitedPane(ctx context.Context, sessionName string, started []startedPane) (*CommandExitedError, error) {
	stdout, _, err := c.runner.Capture(ctx, "list-panes", "-s", "-t", "="+sessionName, "-F", "#{pane_id} #{pane_dead} #{pane_dead_status}")
	if err != nil {
		return nil, fmt.Errorf("tmux list-panes: %w", err)
	}

	byID := make(map[string]startedPane, len(started))
	for _, pane := range started {
		byID[pane.id] = pane
	}
	for line := range strings.Lines(string(stdout)) {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[1] != "1" {
			continue
		}
		pane, ok := byID[fields[0]]
		if !ok {
			continue
		}
		status := -1
		if len(fields) > 2 {
			if parsed, parseErr := strconv.Atoi(fields[2]); parseErr == nil {
				status = parsed
			}
		}
		return &CommandExitedError{
			Session: sessionName,
			Window:  pane.window,
			Command: pane.command,
			Status:  status,
			Output:  c.deadPaneOutput(ctx, pane.id),
		}, nil
	}
	return nil, nil
}

func (c *Client) deadPaneOutput(ctx context.Context, paneID string) string {
	stdout, _, err := c.runner.Capture(ctx, "capture-pane", "-p", "-J", "-t", paneID, "-S", "-20")
	if err != nil {
		c.log.Debug().Err(err).Str("pane", paneID).Msg("failed to capture dead pane output")
		return ""
	}

	var lines []string
	for line := range strings.Lines(string(stdout)) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Pane is dead") {
			continue
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "; ")
}
