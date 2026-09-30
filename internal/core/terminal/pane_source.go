package terminal

import (
	"context"

	"github.com/colonyops/hive/internal/core/multiplexer"
)

// PaneSource supplies pane discovery and capture to terminal status consumers.
type PaneSource interface {
	ListPanes(ctx context.Context) ([]multiplexer.Pane, error)
	CapturePane(ctx context.Context, target multiplexer.Target, opts multiplexer.CaptureOptions) (string, error)
}

// SessionPathKey carries a session path for pane disambiguation.
const SessionPathKey = "_session_path"
