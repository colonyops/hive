package hive

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/hive/events"
	"github.com/colonyops/hive/internal/hive/session/scripts"
	"github.com/colonyops/hive/internal/store/db"
	"github.com/colonyops/hive/pkg/logutils"
)

const (
	busBufferSize   = 64
	kvSweepInterval = 5 * time.Minute
)

// StartStep names the startup step a StartError came from.
type StartStep string

const (
	StartStepDatabase StartStep = "database"
	StartStepEngine   StartStep = "engine"
)

// StartError is a StartRuntime failure. Error returns the cause's message
// unchanged; Step lets a caller add context per step.
type StartError struct {
	Step StartStep
	Err  error
}

func (e *StartError) Error() string { return e.Err.Error() }
func (e *StartError) Unwrap() error { return e.Err }

// RuntimeOptions is what a program hands StartRuntime.
type RuntimeOptions struct {
	// Ports are the program's drivers. DB and Bus must be nil: the runtime
	// opens them.
	Ports Ports
	// ScriptsVersion stamps the bundled scripts extracted into the data dir.
	ScriptsVersion string
}

// Runtime owns the process-lifetime half of the engine: hive.db, the event
// bus and its goroutine, the engine, and the KV sweep. A config reload goes
// through Engine().Reload and keeps all of them.
type Runtime struct {
	engine *Engine
	db     *db.DB
	bus    *events.EventBus
	logger zerolog.Logger

	cancel    context.CancelFunc
	wg        sync.WaitGroup
	closeOnce sync.Once
	closeErr  error
}

// StartRuntime opens hive.db, starts the bus, builds the engine from cfg, and
// starts the KV sweep. Background work stops when ctx is done or on Close. A
// failed step closes everything the earlier steps opened.
func StartRuntime(ctx context.Context, cfg *config.Config, opts RuntimeOptions) (*Runtime, error) {
	p := opts.Ports
	switch {
	case cfg == nil:
		return nil, errors.New("hive runtime: config is required")
	case p.DB != nil || p.Bus != nil:
		return nil, errors.New("hive runtime: DB and Bus are opened by the runtime")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	logger := logutils.Component(p.Logger, "hive-runtime")
	if err := scripts.EnsureExtracted(p.DataDir, opts.ScriptsVersion); err != nil {
		logger.Warn().Err(err).Msg("failed to extract bundled scripts")
	}

	for _, w := range cfg.RemovedSpawnWarnings() {
		logger.Warn().Str("item", w.Item).Msg(w.Message)
	}

	database, err := OpenDB(ctx, cfg.DataDir, cfg.Database)
	if err != nil {
		return nil, &StartError{Step: StartStepDatabase, Err: err}
	}

	bgCtx, cancel := context.WithCancel(ctx)
	r := &Runtime{db: database, bus: events.New(busBufferSize), logger: logger, cancel: cancel}
	r.wg.Go(func() { r.bus.Start(bgCtx) })

	p.DB = database
	p.Bus = r.bus
	r.engine, err = New(cfg, p)
	if err != nil {
		_ = r.Close()
		return nil, &StartError{Step: StartStepEngine, Err: err}
	}

	r.wg.Go(func() { r.engine.SweepKV(bgCtx, kvSweepInterval) })
	return r, nil
}

func (r *Runtime) Engine() *Engine { return r.engine }

// DB returns hive.db. The runtime closes it.
func (r *Runtime) DB() *db.DB { return r.db }

// Bus returns the event bus. Subscribing after StartRuntime is safe.
func (r *Runtime) Bus() *events.EventBus { return r.bus }

// Close stops the drive watcher and the background goroutines, then closes
// hive.db. It is idempotent and returns the first call's error. A drive
// watcher that fails to stop only warns: it must not fail the shutdown.
func (r *Runtime) Close() error {
	r.closeOnce.Do(func() {
		// The drive watcher publishes on the bus, so it stops first.
		if r.engine != nil {
			if err := r.engine.Drive().Close(); err != nil {
				r.logger.Warn().Err(err).Msg("drive watcher shutdown")
			}
		}
		r.cancel()
		r.wg.Wait()
		if err := r.db.Close(); err != nil {
			r.closeErr = fmt.Errorf("close database: %w", err)
		}
	})
	return r.closeErr
}
