package command

import (
	"context"

	coretmux "github.com/colonyops/hive/internal/core/tmux"
)

// TmuxExecutor creates or attaches to a tmux session using window config.
type TmuxExecutor struct {
	builder      *coretmux.Builder
	name         string
	workDir      string
	windows      []coretmux.RenderedWindow
	background   bool
	targetWindow string // specific window to select when opening an existing session
}

var _ Executor = (*TmuxExecutor)(nil)

func (e *TmuxExecutor) Execute(ctx context.Context) (output <-chan string, done <-chan error, cancel context.CancelFunc) {
	ctx, cancel = context.WithCancel(ctx)
	doneCh := make(chan error, 1)

	go func() {
		defer close(doneCh)
		doneCh <- e.builder.OpenSession(ctx, e.name, e.workDir, e.windows, e.background, e.targetWindow)
	}()

	return nil, doneCh, cancel
}
