// Package tmuxexec implements the tmux terminal multiplexer adapter.
package tmuxexec

import (
	"os"

	"github.com/rs/zerolog"
)

// Client implements tmux operations over a Runner.
type Client struct {
	runner Runner
	log    zerolog.Logger
	getenv func(string) string
}

// New creates a tmux client.
func New(runner Runner, log zerolog.Logger) *Client {
	if runner == nil {
		runner = execRunner{}
	}
	return &Client{runner: runner, log: log, getenv: os.Getenv}
}

// NewDefault creates a client that invokes the tmux executable.
func NewDefault(log zerolog.Logger) *Client { return New(execRunner{}, log) }

// Available reports whether tmux is available.
func (c *Client) Available() bool { return c != nil && c.runner != nil && c.runner.Available() }
