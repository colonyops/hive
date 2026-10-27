package command

import (
	"context"
	"fmt"

	"github.com/colonyops/hive/cmd/hive/internal/action"
	"github.com/colonyops/hive/internal/domain/session"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/pkg/executil"
)

// SpawnWindowsExecutor executes a TypeSpawnWindows action.
type SpawnWindowsExecutor struct {
	payload     *action.SpawnWindowsPayload
	adder       WindowAdder
	creator     SessionWindowCreator
	localRemote string

	switchTo *session.Session
}

// SessionToSwitchTo is the session a foreground spawn created. The session
// API may create it in another process, so the TUI switches to it itself.
// Read it only after the done channel delivers.
func (e *SpawnWindowsExecutor) SessionToSwitchTo() *session.Session {
	return e.switchTo
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
		remote := p.NewSessionRemote
		if remote == "" {
			remote = e.localRemote
		}
		req := sessionsvc.NewSessionRequest{Name: p.NewSessionName, Remote: remote, ShCmd: p.ShCmd}
		sess, err := e.creator.CreateSessionWithWindows(ctx, req, p.Windows, p.Background)
		if err != nil {
			return err
		}
		if !p.Background {
			e.switchTo = sess
		}
		return nil
	}

	if p.ShCmd != "" {
		if err := executil.RunSh(ctx, p.ShDir, p.ShCmd); err != nil {
			return fmt.Errorf("sh: %w", err)
		}
	}

	return e.adder.AddWindowsToTmuxSession(ctx, p.TmuxTarget, p.SessionDir, p.Windows, p.Background)
}

var _ Executor = (*SpawnWindowsExecutor)(nil)
