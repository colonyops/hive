package command

import (
	"context"

	"github.com/colonyops/hive/internal/domain/multiplexer"
)

// WindowKiller kills one tmux window for KillWindow actions.
type WindowKiller interface {
	KillTmuxWindow(ctx context.Context, target multiplexer.Target) error
}

type killWindowExecutor struct {
	killer WindowKiller
	target multiplexer.Target
}

func (e *killWindowExecutor) Execute(ctx context.Context) (<-chan string, <-chan error, context.CancelFunc) {
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		defer close(done)
		done <- e.killer.KillTmuxWindow(runCtx, e.target)
	}()
	return nil, done, cancel
}
