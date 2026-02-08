package tui

import (
	"context"

	"github.com/hay-kot/hive/internal/core/git"
	"github.com/hay-kot/hive/internal/core/session"
	"github.com/hay-kot/hive/internal/hive"
)

// SessionLister provides the session listing and git access needed by the TUI Model.
type SessionLister interface {
	ListSessions(ctx context.Context) ([]session.Session, error)
	Git() git.Git
}

// Compile-time check that *hive.Service satisfies SessionLister.
var _ SessionLister = (*hive.Service)(nil)
