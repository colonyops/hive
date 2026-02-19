package command

import (
	"context"
	"fmt"

	"github.com/colonyops/hive/pkg/executil"
)

// ShellExecutor executes a shell command.
type ShellExecutor struct {
	cmd string
	dir string // working directory; empty means inherit hive process cwd
}

// Execute runs the shell command asynchronously.
// Returns nil output channel (non-streaming).
func (e *ShellExecutor) Execute(ctx context.Context) (output <-chan string, done <-chan error, cancel context.CancelFunc) {
	doneCh := make(chan error, 1)
	ctx, cancel = context.WithCancel(ctx)

	go func() {
		defer close(doneCh)
		if err := executil.RunSh(ctx, e.dir, e.cmd); err != nil {
			doneCh <- fmt.Errorf("command failed: %w", err)
			return
		}
		doneCh <- nil
	}()

	return nil, doneCh, cancel
}

var _ Executor = (*ShellExecutor)(nil)
