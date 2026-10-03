package session

import (
	"context"

	"github.com/colonyops/hive/internal/domain/multiplexer"
)

// Lifecycle manages existing multiplexer sessions and windows.
type Lifecycle interface {
	CurrentSession(ctx context.Context) (multiplexer.Target, error)
	AttachOrSwitch(ctx context.Context, target multiplexer.Target, streams multiplexer.AttachStreams) error
	RenameSession(ctx context.Context, target multiplexer.Target, newName string) error
	KillSession(ctx context.Context, target multiplexer.Target) error
	KillWindow(ctx context.Context, target multiplexer.Target) error
}

// Multiplexer combines the creation and lifecycle operations used by Hive.
type Multiplexer interface {
	SessionCreator
	Lifecycle
}
