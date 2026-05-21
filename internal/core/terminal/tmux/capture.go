package tmux

import (
	"context"
	"fmt"
	"os/exec"
)

// TmuxCapture implements classifier.ContentCapture via tmux capture-pane.
type TmuxCapture struct{}

// CapturePane captures content from a tmux pane or target address.
func (TmuxCapture) CapturePane(ctx context.Context, target string) (string, error) {
	output, err := exec.CommandContext(ctx, "tmux", "capture-pane", "-t", target, "-p", "-J").Output()
	if err != nil {
		return "", fmt.Errorf("capture-pane failed: %w", err)
	}
	return string(output), nil
}
