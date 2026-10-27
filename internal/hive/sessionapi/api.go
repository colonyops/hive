// Package sessionapi is the session view the hive TUI, the CLI, and Hive
// Desktop share. Local runs it on an in-process engine; the daemon client
// runs it over RPC.
package sessionapi

import (
	"context"
	"io"

	"github.com/colonyops/hive/internal/domain/multiplexer"
	"github.com/colonyops/hive/internal/domain/session"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/internal/hive/status"
)

type API interface {
	ListSessions(ctx context.Context) ([]session.Session, error)
	GetSession(ctx context.Context, id string) (session.Session, error)
	// CreateSession writes progress to opts.Progress when set. Callers
	// resolve an empty opts.Remote and pass an absolute opts.Source, because
	// the implementation may run in another process.
	CreateSession(ctx context.Context, opts sessionsvc.CreateOptions) (*session.Session, error)
	CreateFromRequest(ctx context.Context, req sessionsvc.LaunchRequest) (session.Session, error)
	CreateSessionWithWindows(ctx context.Context, req sessionsvc.NewSessionRequest, windows []multiplexer.WindowSpec, background bool) (*session.Session, error)
	RenameSession(ctx context.Context, id, name string) error
	SetSessionGroup(ctx context.Context, id, group string) error
	DeleteSession(ctx context.Context, id string) error
	RecycleSession(ctx context.Context, id string, w io.Writer) error
	Prune(ctx context.Context, all bool) (int, error)
	CheckSessionRisk(ctx context.Context, id string) (sessionsvc.Risk, error)
	SessionLaunchOptions(ctx context.Context) (sessionsvc.LaunchOptions, error)

	// StatusAvailable must not block: the TUI calls it inside Update.
	StatusAvailable() bool
	FetchBatch(ctx context.Context, sessions []*session.Session, roots []status.RootRepoTarget) map[string]status.TerminalStatus
}
