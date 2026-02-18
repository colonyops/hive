package command

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
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

		c := exec.CommandContext(ctx, "sh", "-c", e.cmd)
		if e.dir != "" {
			c.Dir = e.dir
		}
		var stderr bytes.Buffer
		c.Stderr = &stderr

		if err := c.Run(); err != nil {
			errMsg := strings.TrimSpace(stderr.String())
			if errMsg != "" {
				doneCh <- fmt.Errorf("command failed: %s", errMsg)
				return
			}
			doneCh <- fmt.Errorf("command failed: %w", err)
			return
		}
		doneCh <- nil
	}()

	return nil, doneCh, cancel
}

var _ Executor = (*ShellExecutor)(nil)
