package sessionapi

import (
	"context"
	"io"

	"github.com/colonyops/hive/internal/domain/multiplexer"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/internal/hive/status"
)

// Local reads the engine's services on every call, so it follows a Reload.
type Local struct {
	engine *hive.Engine
}

var _ API = (*Local)(nil)

func NewLocal(engine *hive.Engine) *Local {
	return &Local{engine: engine}
}

func (l *Local) ListSessions(ctx context.Context) ([]session.Session, error) {
	return l.engine.Sessions().ListSessions(ctx)
}

func (l *Local) GetSession(ctx context.Context, id string) (session.Session, error) {
	return l.engine.Sessions().GetSession(ctx, id)
}

func (l *Local) CreateSession(ctx context.Context, opts sessionsvc.CreateOptions) (*session.Session, error) {
	opts.Background = true
	return l.engine.Sessions().CreateSession(ctx, opts)
}

func (l *Local) CreateFromRequest(ctx context.Context, req sessionsvc.LaunchRequest) (session.Session, error) {
	return l.engine.Sessions().CreateFromRequest(ctx, req)
}

func (l *Local) CreateSessionWithWindows(ctx context.Context, req sessionsvc.NewSessionRequest, windows []multiplexer.WindowSpec) (*session.Session, error) {
	return l.engine.Sessions().CreateSessionWithWindows(ctx, req, windows, true)
}

func (l *Local) RenameSession(ctx context.Context, id, name string) error {
	return l.engine.Sessions().RenameSession(ctx, id, name)
}

func (l *Local) SetSessionGroup(ctx context.Context, id, group string) error {
	return l.engine.Sessions().SetSessionGroup(ctx, id, group)
}

func (l *Local) DeleteSession(ctx context.Context, id string) error {
	return l.engine.Sessions().DeleteSession(ctx, id)
}

func (l *Local) RecycleSession(ctx context.Context, id string, w io.Writer) error {
	return l.engine.Sessions().RecycleSession(ctx, id, w)
}

func (l *Local) Prune(ctx context.Context, all bool) (int, error) {
	return l.engine.Sessions().Prune(ctx, all)
}

func (l *Local) CheckSessionRisk(ctx context.Context, id string) (sessionsvc.Risk, error) {
	return l.engine.Sessions().CheckSessionRisk(ctx, id)
}

func (l *Local) SessionLaunchOptions(ctx context.Context) (sessionsvc.LaunchOptions, error) {
	return l.engine.Sessions().SessionLaunchOptions(ctx)
}

func (l *Local) StatusAvailable() bool {
	return l.engine.Status().Available()
}

func (l *Local) FetchBatch(ctx context.Context, sessions []*session.Session, roots []status.RootRepoTarget) map[string]status.TerminalStatus {
	return l.engine.Status().FetchBatch(ctx, sessions, roots)
}
