package tmuxcc

import (
	"context"
	"fmt"
)

// Commander runs one-shot tmux commands with the binary and server selection
// used by this package's control-mode clients.
type Commander struct {
	locate func() (string, error)
	output func(context.Context, string, ...string) ([]byte, error)
}

// NewCommander builds the one-shot adapter around the app's tmux resolver.
// environ is the same hook ManagerOptions carries, so status detection runs
// with the environment control-mode attaches do; nil means this process's own.
func NewCommander(locate func() (string, error), environ func(context.Context) []string) *Commander {
	return &Commander{
		locate: locate,
		output: func(ctx context.Context, binary string, args ...string) ([]byte, error) {
			var env []string
			if environ != nil {
				env = environ(ctx)
			}
			return outputTmux(ctx, binary, env, args...)
		},
	}
}

func (c *Commander) Available() bool {
	if c == nil || c.locate == nil {
		return false
	}
	_, err := c.locate()
	return err == nil
}

func (c *Commander) Output(ctx context.Context, args ...string) ([]byte, error) {
	if c == nil || c.locate == nil {
		return nil, fmt.Errorf("tmux commander is unavailable")
	}
	binary, err := c.locate()
	if err != nil {
		return nil, err
	}
	return c.output(ctx, binary, args...)
}
