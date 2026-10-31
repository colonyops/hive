package app

import (
	"cmp"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/domain/server"
	"github.com/colonyops/hive/internal/hive/daemon/daemonpath"
	"github.com/colonyops/hive/internal/hive/servers"
	"github.com/colonyops/hive/internal/platform/tmux/control"
)

const terminalStopBudget = 3 * time.Second

// serverTerminals holds one tmux control manager per server, each running the
// server's tmux over ssh. A manager is built on the first terminal call for its
// server, and a config reload that changes or removes the server stops it.
type serverTerminals struct {
	fleet      func() *servers.Fleet
	newManager func(name string, host server.Host) *tmuxcc.Manager

	mu       sync.Mutex
	managers map[string]serverManager
	stopped  bool
}

type serverManager struct {
	host    server.Host
	manager *tmuxcc.Manager
}

// sshControlDir is the private directory ssh keeps its shared connections in,
// or "" to give each command its own connection when there is none. It sits
// in /tmp because a unix socket path must fit in about 100 bytes.
func sshControlDir(log zerolog.Logger) string {
	root, err := daemonpath.PrivateTempDir()
	if err == nil {
		dir := filepath.Join(root, "ssh")
		if err = os.Mkdir(dir, 0o700); err == nil || errors.Is(err, os.ErrExist) {
			return dir
		}
	}
	log.Warn().Err(err).Msg("no directory for shared ssh connections; each server terminal command opens its own")
	return ""
}

// newServerTerminals builds the managers under ctx, the app's lifetime.
// controlDir is where ssh shares one connection for each server.
func newServerTerminals(ctx context.Context, log zerolog.Logger, fleet func() *servers.Fleet, environ func(context.Context) []string, controlDir string) *serverTerminals {
	return &serverTerminals{
		fleet: fleet,
		newManager: func(name string, host server.Host) *tmuxcc.Manager {
			return tmuxcc.NewManager(ctx, tmuxcc.ManagerOptions{
				Logger:  log.With().Str("server", name).Logger(),
				Binary:  func() (string, error) { return cmp.Or(host.Tmux, "tmux"), nil },
				Environ: environ,
				Runner:  tmuxcc.SSHRunner{Host: host, ControlDir: controlDir},
			})
		},
		managers: map[string]serverManager{},
	}
}

// Manager is the tmux manager for the named server.
func (t *serverTerminals) Manager(ctx context.Context, name string) (*tmuxcc.Manager, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stopped {
		return nil, Wrap(tmuxcc.ErrUnavailable, KindUnavailable, "the app is shutting down")
	}
	// Under the lock: a reload swaps the fleet and then prunes, so a fleet read
	// before the lock could store a manager for a host the prune already passed.
	fleet := t.fleet()
	if fleet == nil {
		return nil, Errorf(KindUnavailable, "the server list is not loaded")
	}
	srv, ok := fleet.Server(name)
	if !ok {
		return nil, Errorf(KindNotFound, "server %q is not under servers: in config.yaml", name)
	}
	host := srv.Config().Host()
	if current, ok := t.managers[name]; ok {
		if current.host == host {
			return current.manager, nil
		}
		t.stopLocked(ctx, name, current)
	}
	manager := t.newManager(name, host)
	t.managers[name] = serverManager{host: host, manager: manager}
	return manager, nil
}

// Prune stops the managers of servers the fleet no longer has, or has
// with a different ssh target.
func (t *serverTerminals) Prune(ctx context.Context) {
	t.mu.Lock()
	defer t.mu.Unlock()
	fleet := t.fleet()
	for name, current := range t.managers {
		var srv *servers.Server
		if fleet != nil {
			srv, _ = fleet.Server(name)
		}
		if srv == nil || srv.Config().Host() != current.host {
			t.stopLocked(ctx, name, current)
		}
	}
}

// stopLocked stops the manager in the background, since closing its clients
// waits on their ssh processes.
func (t *serverTerminals) stopLocked(ctx context.Context, name string, current serverManager) {
	delete(t.managers, name)
	go func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), terminalStopBudget)
		defer cancel()
		_ = current.manager.Stop(ctx)
	}()
}

// Stop closes every server's terminals within ctx.
func (t *serverTerminals) Stop(ctx context.Context) {
	t.mu.Lock()
	t.stopped = true
	managers := t.managers
	t.managers = map[string]serverManager{}
	t.mu.Unlock()

	var wg sync.WaitGroup
	for _, current := range managers {
		wg.Go(func() { _ = current.manager.Stop(ctx) })
	}
	wg.Wait()
}
