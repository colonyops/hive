package tmuxstatus

import (
	"context"
	"fmt"
)

// TmuxCapture captures a pane by raw tmux target string. The assess commands
// use it because they accept any tmux target syntax from the user.
type TmuxCapture struct {
	commander Commander
}

// CapturePane captures content from a tmux pane or target address.
func (c TmuxCapture) CapturePane(ctx context.Context, target string) (string, error) {
	commander := c.commander
	if commander == nil {
		commander = execCommander{}
	}
	output, err := commander.Output(ctx, "capture-pane", "-t", target, "-p", "-J")
	if err != nil {
		return "", fmt.Errorf("capture-pane failed: %w", err)
	}
	return string(output), nil
}
