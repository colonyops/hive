package command

import (
	"context"
)

// RecycleExecutor executes a session recycle operation with streaming output.
type RecycleExecutor struct {
	recycler  SessionRecycler
	sessionID string
}

// Execute starts the recycle operation and returns channels for output and completion.
// Returns non-nil output channel with streaming recycle progress.
func (e *RecycleExecutor) Execute(ctx context.Context) (output <-chan string, done <-chan error, cancel context.CancelFunc) {
	outCh := make(chan string, 100)
	doneCh := make(chan error, 1)

	ctx, cancel = context.WithCancel(ctx)

	go func() {
		defer close(outCh)
		defer close(doneCh)

		writer := &channelWriter{ch: outCh, ctx: ctx}
		err := e.recycler.RecycleSession(ctx, e.sessionID, writer)
		doneCh <- err
	}()

	return outCh, doneCh, cancel
}

var _ Executor = (*RecycleExecutor)(nil)
