package tmux

import "github.com/colonyops/hive/internal/core/terminal"

// paneState holds mutable polling state for an agent pane.
type paneState struct {
	paneContent       string
	cachedStatus      terminal.Status
	lastCaptureActive int64
}
