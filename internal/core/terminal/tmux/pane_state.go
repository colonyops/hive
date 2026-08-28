package tmux

// paneState holds mutable polling state for an agent pane.
type paneState struct {
	paneContent       string
	lastCaptureActive int64
}
