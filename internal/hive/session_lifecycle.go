package hive

import (
	"context"

	"github.com/colonyops/hive/internal/core/multiplexer"
)

// SessionLifecycle manages existing multiplexer sessions and windows.
type SessionLifecycle interface {
	CurrentSession(ctx context.Context) (multiplexer.Target, error)
	AttachOrSwitch(ctx context.Context, target multiplexer.Target, streams multiplexer.AttachStreams) error
	RenameSession(ctx context.Context, target multiplexer.Target, newName string) error
	KillSession(ctx context.Context, target multiplexer.Target) error
	KillWindow(ctx context.Context, target multiplexer.Target) error
}

// SessionMultiplexer combines the creation and lifecycle operations used by Hive.
type SessionMultiplexer interface {
	SessionCreator
	SessionLifecycle
}
