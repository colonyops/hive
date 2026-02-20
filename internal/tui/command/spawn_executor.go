package command

import (
	"context"
	"fmt"

	"github.com/colonyops/hive/internal/core/action"
	"github.com/colonyops/hive/pkg/executil"
)

// SpawnWindowsExecutor executes a TypeSpawnWindows action.
type SpawnWindowsExecutor struct {
	payload *action.SpawnWindowsPayload
	spawner WindowSpawner
}

// Execute runs the SpawnWindows action asynchronously.
func (e *SpawnWindowsExecutor) Execute(ctx context.Context) (<-chan string, <-chan error, context.CancelFunc) {
	doneCh := make(chan error, 1)
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		defer close(doneCh)
		doneCh <- e.run(ctx)
	}()
	return nil, doneCh, cancel
}

func (e *SpawnWindowsExecutor) run(ctx context.Context) error {
	p := e.payload

	if p.NewSession != nil && p.ShCmd != "" {
		// Invariant: sh: is routed to NewSession.ShCmd by resolveWindowsAction.
		// A non-empty top-level ShCmd with NewSession set would be silently ignored.
		return fmt.Errorf("internal error: SpawnWindowsPayload.ShCmd must be empty when NewSession is set")
	}

	// New-session mode: create Hive session first.
	// sh: (if any) runs inside CreateSessionWithWindows after the git clone.
	if p.NewSession != nil {
		return e.spawner.CreateSessionWithWindows(ctx, *p.NewSession, p.Windows, p.Background)
	}

	// Same-session mode: optionally run sh: in the selected session's path, then add windows.
	if p.ShCmd != "" {
		if err := executil.RunSh(ctx, p.ShDir, p.ShCmd); err != nil {
			return fmt.Errorf("sh: %w", err)
		}
	}

	return e.spawner.AddWindowsToTmuxSession(ctx, p.TmuxTarget, p.SessionDir, p.Windows, p.Background)
}

var _ Executor = (*SpawnWindowsExecutor)(nil)
