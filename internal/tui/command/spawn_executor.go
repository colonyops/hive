package command

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/colonyops/hive/internal/core/action"
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

	// New-session mode: create Hive session first.
	// sh: (if any) runs inside CreateSessionWithWindows after the git clone.
	if p.NewSession != nil {
		return e.spawner.CreateSessionWithWindows(ctx, *p.NewSession, p.Windows, p.Background)
	}

	// Same-session mode: optionally run sh: in the selected session's path, then add windows.
	if p.ShCmd != "" {
		c := exec.CommandContext(ctx, "sh", "-c", p.ShCmd)
		if p.ShDir != "" {
			c.Dir = p.ShDir
		}
		var stderr bytes.Buffer
		c.Stdout = io.Discard
		c.Stderr = &stderr
		if err := c.Run(); err != nil {
			msg := strings.TrimSpace(stderr.String())
			if msg != "" {
				return fmt.Errorf("sh: %s", msg)
			}
			return fmt.Errorf("sh: %w", err)
		}
	}

	return e.spawner.AddWindowsToTmuxSession(ctx, p.TmuxTarget, p.SessionDir, p.Windows, p.Background)
}

var _ Executor = (*SpawnWindowsExecutor)(nil)
