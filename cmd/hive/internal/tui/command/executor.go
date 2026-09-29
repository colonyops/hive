package command

import "context"

// Executor executes a command, optionally with streaming output.
type Executor interface {
	// Execute runs the command asynchronously. Returns:
	// - output: streaming output lines (nil for non-streaming commands)
	// - done: receives error (or nil) when complete
	// - cancel: cancels the operation
	Execute(ctx context.Context) (output <-chan string, done <-chan error, cancel context.CancelFunc)
}

// ExecuteSync runs an executor synchronously, blocking until completion.
// Streaming output is discarded. Returns the final error.
func ExecuteSync(ctx context.Context, exec Executor) error {
	output, done, cancel := exec.Execute(ctx)
	defer cancel()

	// Drain output channel if present
	if output != nil {
		go func() {
			for range output {
			}
		}()
	}

	return <-done
}
