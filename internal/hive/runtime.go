package hive

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/terminal"
	"github.com/colonyops/hive/internal/hive/events"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/internal/hive/session/scripts"
	"github.com/colonyops/hive/internal/store/db"
	"github.com/colonyops/hive/pkg/executil"
)

const (
	busBufferSize   = 64
	kvSweepInterval = 5 * time.Minute
)

// RuntimeOptions are the program-specific drivers and policy a Runtime hands
// the engine. The runtime opens hive.db and the event bus itself.
type RuntimeOptions struct {
	Executor executil.Executor
	Mux      sessionsvc.Multiplexer
	// PaneSource feeds terminal status. Nil disables status.
	PaneSource terminal.PaneSource
	Styler     sessionsvc.OutputStyler
	Stdout     io.Writer
	Stderr     io.Writer
	Logger     zerolog.Logger
	// ScriptsVersion stamps the bundled scripts extracted into the data dir. A
	// different stamp re-extracts them.
	ScriptsVersion string
}

// StartupStep names the Open step that failed.
type StartupStep string

const (
	StartupStepDatabase StartupStep = "database"
	StartupStepEngine   StartupStep = "engine"
)

// StartupError is the error Open returns when a step fails. Its message is
// the underlying error's, so a program that does not care about the step
// reports the same text it did before the runtime existed.
type StartupError struct {
	Step StartupStep
	Err  error
}

func (e *StartupError) Error() string { return e.Err.Error() }
func (e *StartupError) Unwrap() error { return e.Err }

// Runtime owns the process-lifetime half of the hive engine: hive.db, the
// event bus, the engine built over them, and the background maintenance they
// need. Config-derived services are the engine's to rebuild on Reload; the
// runtime's resources stay open until Close.
type Runtime struct {
	engine *Engine
	db     *db.DB
	cancel context.CancelFunc
	wg     sync.WaitGroup

	closeOnce sync.Once
	closeErr  error
}

// Open starts the runtime for cfg. On error, everything Open opened is closed
// again. Cancelling ctx stops the background work; Close is still required to
// release the database.
func Open(ctx context.Context, cfg *config.Config, opts RuntimeOptions) (*Runtime, error) {
	if cfg == nil {
		return nil, errors.New("hive runtime: config is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("hive runtime: %w", err)
	}

	if err := scripts.EnsureExtracted(cfg.DataDir, opts.ScriptsVersion); err != nil {
		opts.Logger.Warn().Err(err).Msg("extract bundled scripts failed")
	}

	database, err := OpenDB(ctx, cfg.DataDir, cfg.Database)
	if err != nil {
		return nil, &StartupError{Step: StartupStepDatabase, Err: err}
	}

	bus := events.New(busBufferSize)
	events.RegisterDebugLogger(opts.Logger, bus)
	NewNotificationRouter(bus).Register()

	engine, err := New(cfg, Ports{
		DB:         database,
		Bus:        bus,
		Executor:   opts.Executor,
		Mux:        opts.Mux,
		PaneSource: opts.PaneSource,
		DataDir:    cfg.DataDir,
		Styler:     opts.Styler,
		Stdout:     opts.Stdout,
		Stderr:     opts.Stderr,
		Logger:     opts.Logger,
	})
	if err != nil {
		_ = database.Close()
		return nil, &StartupError{Step: StartupStepEngine, Err: err}
	}

	bgCtx, cancel := context.WithCancel(ctx)
	r := &Runtime{engine: engine, db: database, cancel: cancel}
	r.wg.Go(func() { bus.Start(bgCtx) })
	r.wg.Go(func() { engine.SweepKV(bgCtx, kvSweepInterval) })
	return r, nil
}

func (r *Runtime) Engine() *Engine { return r.engine }

// DB returns hive.db for callers that need the raw handle, such as test
// harnesses. The runtime closes it.
func (r *Runtime) DB() *db.DB { return r.db }

// Close stops the event bus and the maintenance loops, waits for them, and
// then closes hive.db. Calls after the first return the first call's error.
func (r *Runtime) Close() error {
	r.closeOnce.Do(func() {
		r.cancel()
		r.wg.Wait()
		r.closeErr = r.db.Close()
	})
	return r.closeErr
}
