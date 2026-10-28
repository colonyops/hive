// Package hivetest builds a hive.Engine over a temp database, a recording
// executor, and a tmux stand-in, for tests in any package that needs one.
package hivetest

import (
	"context"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/multiplexer"
	"github.com/colonyops/hive/internal/domain/terminal"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/hive/events/testbus"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/internal/store/db"
	"github.com/colonyops/hive/pkg/executil/executiltest"
)

// Mux is a multiplexer that succeeds at everything and does nothing.
type Mux struct{}

func (Mux) CreateSession(context.Context, multiplexer.SessionSpec) error { return nil }
func (Mux) OpenSession(context.Context, multiplexer.SessionSpec, multiplexer.Target) (multiplexer.LaunchResult, error) {
	return multiplexer.LaunchResult{Created: true}, nil
}

func (Mux) AddWindows(context.Context, multiplexer.Target, []multiplexer.WindowSpec) error {
	return nil
}

func (Mux) AttachOrSwitch(context.Context, multiplexer.Target, multiplexer.AttachStreams) error {
	return nil
}

func (Mux) CurrentSession(context.Context) (multiplexer.Target, error) {
	return multiplexer.Target{}, nil
}
func (Mux) RenameSession(context.Context, multiplexer.Target, string) error { return nil }
func (Mux) KillSession(context.Context, multiplexer.Target) error           { return nil }
func (Mux) KillWindow(context.Context, multiplexer.Target) error            { return nil }

var _ sessionsvc.Multiplexer = Mux{}

// PaneSource reports no panes. Passing one turns the engine's status service on.
type PaneSource struct{}

func (PaneSource) ListPanes(context.Context) ([]multiplexer.Pane, error) { return nil, nil }

func (PaneSource) CapturePane(context.Context, multiplexer.Target, multiplexer.CaptureOptions) (string, error) {
	return "", nil
}

var _ terminal.PaneSource = PaneSource{}

// LoadConfig loads the default config over dataDir, with no default agent
// leaking in from the environment.
func LoadConfig(t *testing.T, dataDir string) *config.Config {
	t.Helper()
	t.Setenv("HIVE_DEFAULT_AGENT", "")
	cfg, err := config.Load("", dataDir)
	require.NoError(t, err)
	return cfg
}

type Options struct {
	// Config defaults to LoadConfig over a temp dir.
	Config *config.Config
	// PaneSource nil leaves the engine with no status service.
	PaneSource terminal.PaneSource
	// Mux defaults to Mux{}.
	Mux sessionsvc.Multiplexer
}

type Harness struct {
	Engine *hive.Engine
	DB     *db.DB
	Exec   *executiltest.Exec
	Bus    *testbus.Bus
}

func NewEngine(t *testing.T, opts Options) Harness {
	t.Helper()
	cfg := opts.Config
	if cfg == nil {
		cfg = LoadConfig(t, t.TempDir())
	}
	mux := opts.Mux
	if mux == nil {
		mux = Mux{}
	}

	database, err := hive.OpenDB(t.Context(), t.TempDir(), cfg.Database)
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })

	h := Harness{DB: database, Exec: &executiltest.Exec{}, Bus: testbus.New(t)}
	h.Engine, err = hive.New(cfg, hive.Ports{
		DB:         database,
		Bus:        h.Bus.EventBus,
		Executor:   h.Exec,
		Mux:        mux,
		PaneSource: opts.PaneSource,
		DataDir:    t.TempDir(),
		Logger:     zerolog.Nop(),
	})
	require.NoError(t, err)
	return h
}
