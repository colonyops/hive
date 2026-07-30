package tmux

import (
	"context"
	"os/exec"
)

// Commander runs tmux commands for an Integration. Implementations may select
// an absolute binary, environment, or server socket for embedded use. Methods
// may be called concurrently and must be safe for concurrent use.
type Commander interface {
	Available() bool
	Output(ctx context.Context, args ...string) ([]byte, error)
}

type execCommander struct{}

func (execCommander) Available() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

func (execCommander) Output(ctx context.Context, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, "tmux", args...).Output()
}
