package tmuxexec

import (
	"context"
	"fmt"
	"strings"
)

func (c *Client) recoverLaunch(ctx context.Context, name string) (bool, error) {
	target := "=" + name
	marker, _, err := c.runner.Capture(ctx, "show-options", "-q", "-v", "-t", target+":", launchMarker)
	if err != nil {
		return true, fmt.Errorf("inspect previous tmux launch: %w", err)
	}
	if strings.TrimSpace(string(marker)) != "" {
		id, _, err := c.runner.Capture(ctx, "display-message", "-p", "-t", target+":", "#{session_id}")
		if err != nil {
			return true, fmt.Errorf("resolve interrupted tmux session: %w", err)
		}
		sessionID := strings.TrimSpace(string(id))
		if !validNativeID(sessionID, '$') {
			return true, fmt.Errorf("interrupted tmux launch returned no session ID")
		}
		if _, _, err := c.runner.Capture(ctx, "kill-session", "-t", sessionID); err != nil {
			return true, fmt.Errorf("remove interrupted tmux session %s: %w", sessionID, err)
		}
		return false, nil
	}
	stdout, _, err := c.runner.Capture(ctx, "list-windows", "-t", target, "-F", "#{window_id}|#{"+launchMarker+"}")
	if err != nil {
		return true, fmt.Errorf("inspect interrupted tmux windows: %w", err)
	}
	for line := range strings.Lines(string(stdout)) {
		id, pending, ok := strings.Cut(strings.TrimSuffix(line, "\n"), "|")
		if !ok || !validNativeID(id, '@') {
			return true, fmt.Errorf("invalid tmux window launch metadata %q", line)
		}
		if pending == "" {
			continue
		}
		if _, _, err := c.runner.Capture(ctx, "kill-window", "-t", id); err != nil {
			return true, fmt.Errorf("remove interrupted tmux window %s: %w", id, err)
		}
	}
	return true, nil
}
