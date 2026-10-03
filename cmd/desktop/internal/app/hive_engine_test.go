package app

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/multiplexer"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/domain/terminal"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/hive/events"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/internal/store"
	"github.com/colonyops/hive/pkg/executil"
	"github.com/colonyops/hive/pkg/executil/executiltest"
)

// fakeMux records what the session service asks of tmux without one running.
type fakeMux struct {
	mu      sync.Mutex
	renamed [][2]string
	killed  []string
	opened  []multiplexer.SessionSpec
}

var _ sessionsvc.Multiplexer = (*fakeMux)(nil)

func (m *fakeMux) CreateSession(_ context.Context, spec multiplexer.SessionSpec) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.opened = append(m.opened, spec)
	return nil
}

func (m *fakeMux) OpenSession(_ context.Context, spec multiplexer.SessionSpec, _ multiplexer.Target) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.opened = append(m.opened, spec)
	return nil
}

func (m *fakeMux) AddWindows(context.Context, multiplexer.Target, []multiplexer.WindowSpec) error {
	return nil
}

func (m *fakeMux) AttachOrSwitch(context.Context, multiplexer.Target, multiplexer.AttachStreams) error {
	return nil
}

func (m *fakeMux) CurrentSession(context.Context) (multiplexer.Target, error) {
	return multiplexer.Target{}, nil
}

func (m *fakeMux) RenameSession(_ context.Context, target multiplexer.Target, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.renamed = append(m.renamed, [2]string{target.Session, name})
	return nil
}

func (m *fakeMux) KillSession(_ context.Context, target multiplexer.Target) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.killed = append(m.killed, target.Session)
	return nil
}

func (m *fakeMux) KillWindow(context.Context, multiplexer.Target) error { return nil }

// stubPanes turns status detection on with no panes, so every active session
// reads as missing to hive and liveness comes from the window source.
type stubPanes struct{}

func (stubPanes) ListPanes(context.Context) ([]multiplexer.Pane, error) { return nil, nil }

func (stubPanes) CapturePane(context.Context, multiplexer.Target, multiplexer.CaptureOptions) (string, error) {
	return "", nil
}

type engineOptions struct {
	cfg   func(*config.Config)
	mux   sessionsvc.Multiplexer
	exec  executil.Executor
	panes terminal.PaneSource
}

type hiveHarness struct {
	engine *hive.Engine
	mux    *fakeMux
	exec   *executiltest.Exec
	cfg    *config.Config
}

// newHiveHarness builds a real hive engine over a temporary hive.db. tmux is a
// fake and every subprocess answers empty and successfully unless opts says
// otherwise.
func newHiveHarness(t *testing.T, opts engineOptions) *hiveHarness {
	t.Helper()
	t.Setenv("HIVE_DEFAULT_AGENT", "")
	dataDir := t.TempDir()
	cfg, err := config.Load("", dataDir)
	require.NoError(t, err)
	if opts.cfg != nil {
		opts.cfg(cfg)
	}

	database, err := hive.OpenDB(t.Context(), dataDir, cfg.Database)
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })

	h := &hiveHarness{mux: &fakeMux{}, exec: &executiltest.Exec{}, cfg: cfg}
	mux := opts.mux
	if mux == nil {
		mux = h.mux
	}
	exec := opts.exec
	if exec == nil {
		exec = h.exec
	}
	h.engine, err = hive.New(cfg, hive.Ports{
		DB:         database,
		Bus:        events.New(8),
		Executor:   exec,
		Mux:        mux,
		PaneSource: opts.panes,
		DataDir:    dataDir,
		Logger:     zerolog.Nop(),
	})
	require.NoError(t, err)
	return h
}

// save writes s to hive.db, filling the timestamps a real session always has.
func (h *hiveHarness) save(t *testing.T, sessions ...session.Session) {
	t.Helper()
	sessionStore := store.NewSessionStore(h.engine.DB())
	now := time.Now().UTC().Truncate(time.Second)
	for _, s := range sessions {
		if s.CreatedAt.IsZero() {
			s.CreatedAt, s.UpdatedAt = now, now
		}
		require.NoError(t, sessionStore.Save(t.Context(), s))
	}
}

// reviewSession is the active session most tests act on.
func reviewSession() session.Session {
	return session.Session{ID: "s1", Name: "review 81", Slug: "review-81", Remote: "acme/site", State: session.StateActive, CloneStrategy: session.CloneStrategyFull}
}
