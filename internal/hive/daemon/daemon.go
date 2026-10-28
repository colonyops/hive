// Package daemon serves sessionapi.API for one hive data directory on a unix
// socket, so every client on the machine shares one writer and one status
// tracker.
package daemon

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/hive/daemon/gen/hive/daemon/v1/daemonv1connect"
	"github.com/colonyops/hive/internal/hive/sessionapi"
	"github.com/colonyops/hive/pkg/flock"
	"github.com/colonyops/hive/pkg/logutils"
)

var ErrAlreadyRunning = errors.New("hive daemon already running")

const shutdownTimeout = 10 * time.Second

type Options struct {
	Config *config.Config
	// Ports are the daemon's drivers. DB and Bus must be nil: Run opens them.
	// Stdout and Stderr should go to the log, since no terminal watches.
	Ports   hive.Ports
	Version string
	// WaitLock blocks until the running daemon exits instead of returning
	// ErrAlreadyRunning. The OS service units use it, so a unit restart
	// queues behind a daemon a client spawned.
	WaitLock bool
	// Reload loads the config a Reload RPC applies. nil rejects the RPC.
	Reload func(ctx context.Context) (*config.Config, error)
	// OnReady runs once the socket accepts connections.
	OnReady func(socketPath string)
}

// Run serves until ctx ends or a client calls Stop. It holds the instance
// lock for its whole life and removes the socket on the way out.
func Run(ctx context.Context, opts Options) error {
	if opts.Config == nil {
		return errors.New("hive daemon: config is required")
	}
	logger := logutils.Component(opts.Ports.Logger, "daemon")
	dataDir := absolute(opts.Config.DataDir)
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}

	unlock, err := flock.Lock(ctx, LockPath(dataDir), opts.WaitLock)
	if errors.Is(err, flock.ErrLocked) {
		return ErrAlreadyRunning
	}
	if err != nil {
		return fmt.Errorf("take the daemon lock: %w", err)
	}
	defer func() {
		if err := unlock(); err != nil {
			logger.Warn().Err(err).Msg("release the daemon lock")
		}
	}()

	socketPath := SocketPath(dataDir)
	if err := prepareSocketDir(socketPath); err != nil {
		return err
	}
	// Holding the lock means no daemon serves this socket, so a leftover file
	// is from one that died.
	if err := os.Remove(socketPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove stale socket: %w", err)
	}

	runtime, err := hive.StartRuntime(ctx, opts.Config, hive.RuntimeOptions{Ports: opts.Ports, ScriptsVersion: opts.Version})
	if err != nil {
		return err
	}
	defer func() {
		if err := runtime.Close(); err != nil {
			logger.Warn().Err(err).Msg("close the hive runtime")
		}
	}()
	hive.NewNotificationRouter(runtime.Bus()).Register()

	runCtx, stop := context.WithCancel(ctx)
	defer stop()

	engine := runtime.Engine()
	api := sessionapi.NewLocal(engine)
	loop := newStatusLoop(statusLoopDeps{
		logger:    logger,
		interval:  func() time.Duration { return engine.Config().Tmux.PollInterval },
		available: api.StatusAvailable,
		list:      api.ListSessions,
		fetch:     api.FetchBatch,
	})
	go loop.run(runCtx)
	sessions := &sessionServer{api: api, logger: logger, statuses: loop.snapshot}
	control := &controlServer{
		runtime:   runtime,
		api:       api,
		paneOK:    opts.Ports.PaneSource,
		version:   opts.Version,
		dataDir:   dataDir,
		startedAt: time.Now(),
		reload:    opts.Reload,
		stop:      stop,
		logger:    logger,
		loop:      loop.health,
	}

	mux := http.NewServeMux()
	mux.Handle(daemonv1connect.NewSessionServiceHandler(sessions))
	mux.Handle(daemonv1connect.NewDaemonServiceHandler(control))
	mux.HandleFunc("GET /healthz", healthz)

	listener, err := listen(socketPath)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(socketPath) }()

	server := newServer(mux)
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()
	logger.Info().Str("socket", socketPath).Int("pid", os.Getpid()).Str("version", opts.Version).Msg("daemon listening")
	if opts.OnReady != nil {
		opts.OnReady(socketPath)
	}

	select {
	case err := <-serveErr:
		return fmt.Errorf("serve: %w", err)
	case <-runCtx.Done():
	}

	logger.Info().Msg("daemon stopping")
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Warn().Err(err).Msg("in-flight requests did not finish before shutdown")
	}
	return nil
}

func newServer(handler http.Handler) *http.Server {
	var protocols http.Protocols
	protocols.SetUnencryptedHTTP2(true)
	return &http.Server{
		Handler:           handler,
		Protocols:         &protocols,
		ReadHeaderTimeout: 10 * time.Second,
	}
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok"))
}

// listen opens the socket with a 0077 umask, so no window exists in which
// another user can connect before the chmod to 0600.
func listen(socketPath string) (net.Listener, error) {
	restore := setUmask(0o077)
	listener, err := net.Listen("unix", socketPath)
	restore()
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", socketPath, err)
	}
	if err := os.Chmod(socketPath, 0o600); err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("restrict socket: %w", err)
	}
	return listener, nil
}
