package command

import (
	"context"

	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/hive"
)

// SessionCreator is the interface for creating sessions.
type SessionCreator interface {
	CreateSession(ctx context.Context, opts hive.CreateOptions) (*session.Session, error)
}

// CreateExecutor executes a session creation with streaming output.
type CreateExecutor struct {
	creator SessionCreator
	opts    hive.CreateOptions

	// ResultSessionID and ResultSessionName are populated with the created
	// session's ID and name before the done channel fires. Safe to read
	// after receiving from done.
	ResultSessionID   string
	ResultSessionName string
}

// Execute starts the create operation and returns channels for output and completion.
func (e *CreateExecutor) Execute(ctx context.Context) (output <-chan string, done <-chan error, cancel context.CancelFunc) {
	outCh := make(chan string, 100)
	doneCh := make(chan error, 1)

	ctx, cancel = context.WithCancel(ctx)

	go func() {
		defer close(outCh)
		defer close(doneCh)

		opts := e.opts
		opts.Progress = &channelWriter{ch: outCh, ctx: ctx}
		sess, err := e.creator.CreateSession(ctx, opts)
		if err == nil && sess != nil {
			e.ResultSessionID = sess.ID
			e.ResultSessionName = sess.Name
		}
		doneCh <- err
	}()

	return outCh, doneCh, cancel
}

var _ Executor = (*CreateExecutor)(nil)
