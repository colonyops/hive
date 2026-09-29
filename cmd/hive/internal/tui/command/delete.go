package command

import "context"

// DeleteExecutor executes a session delete operation.
type DeleteExecutor struct {
	deleter   SessionDeleter
	sessionID string
}

// Execute deletes the session asynchronously.
// Returns nil output channel (non-streaming).
func (e *DeleteExecutor) Execute(ctx context.Context) (output <-chan string, done <-chan error, cancel context.CancelFunc) {
	doneCh := make(chan error, 1)
	ctx, cancel = context.WithCancel(ctx)

	go func() {
		defer close(doneCh)
		doneCh <- e.deleter.DeleteSession(ctx, e.sessionID)
	}()

	return nil, doneCh, cancel
}

var _ Executor = (*DeleteExecutor)(nil)
