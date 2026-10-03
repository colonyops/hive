package tmux

import (
	"context"

	"github.com/colonyops/hive/internal/domain/multiplexer"
	"github.com/colonyops/hive/internal/domain/terminal"
)

// PaneCapture adapts a terminal.PaneSource to classifier.ContentCapture.
// Wrapped lines are joined so prompt and status patterns match regardless of
// pane width.
type PaneCapture struct{ Source terminal.PaneSource }

// CapturePane returns the visible content of one pane.
func (c PaneCapture) CapturePane(ctx context.Context, target multiplexer.Target) (string, error) {
	return c.Source.CapturePane(ctx, target, multiplexer.CaptureOptions{JoinWrappedLines: true})
}
