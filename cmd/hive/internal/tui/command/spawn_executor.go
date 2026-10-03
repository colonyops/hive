package command

import (
	"context"
	"fmt"

	"github.com/colonyops/hive/cmd/hive/internal/action"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
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

	if p.NewSession {
		req := sessionsvc.NewSessionRequest{Name: p.NewSessionName, Remote: p.NewSessionRemote, ShCmd: p.ShCmd}
		return e.spawner.CreateSessionWithWindows(ctx, req, p.Windows, p.Background)
	}

	if p.ShCmd != "" {
		if err := executil.RunSh(ctx, p.ShDir, p.ShCmd); err != nil {
			return fmt.Errorf("sh: %w", err)
		}
	}

	return e.spawner.AddWindowsToTmuxSession(ctx, p.TmuxTarget, p.SessionDir, p.Windows, p.Background)
}

var _ Executor = (*SpawnWindowsExecutor)(nil)
