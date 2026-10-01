package executil

import (
	"context"
	"sync"
)

// RecordedCommand captures a command that was executed.
type RecordedCommand struct {
	Dir  string
	Cmd  string
	Args []string
}

// RecordingExecutor captures commands for testing.
// Configure Outputs and Errors maps to control return values.
type RecordingExecutor struct {
	mu       sync.Mutex
	Commands []RecordedCommand

	// Outputs maps command names to their output.
	// Key is the command name (e.g., "git").
	Outputs map[string][]byte

	// Errors maps command names to their error.
	Errors map[string]error
}

// Run records the command and returns configured output/error.
func (e *RecordingExecutor) Run(ctx context.Context, cmd string, args ...string) ([]byte, error) {
	return e.record("", cmd, args...)
}

// RunDir records the command with directory and returns configured output/error.
func (e *RecordingExecutor) RunDir(ctx context.Context, dir, cmd string, args ...string) ([]byte, error) {
	return e.record(dir, cmd, args...)
}

func (e *RecordingExecutor) record(dir, cmd string, args ...string) ([]byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.Commands = append(e.Commands, RecordedCommand{
		Dir:  dir,
		Cmd:  cmd,
		Args: args,
	})

	var out []byte
	var err error

	if e.Outputs != nil {
		out = e.Outputs[cmd]
	}
	if e.Errors != nil {
		err = e.Errors[cmd]
	}

	return out, err
}

// Reset clears recorded commands.
func (e *RecordingExecutor) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Commands = nil
}
