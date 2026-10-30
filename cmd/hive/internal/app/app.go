// Package app is the hive CLI's composition root: the engine services plus the
// pieces only the CLI has (plugins, the command set, sources, its config).
package app

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/hive/internal/config"
	"github.com/colonyops/hive/cmd/hive/internal/plugins"
	"github.com/colonyops/hive/cmd/hive/internal/sources"
	hiveconfig "github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/multiplexer"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/domain/terminal"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/hive/daemonclient"
	"github.com/colonyops/hive/internal/hive/doctor"
	"github.com/colonyops/hive/internal/hive/servers"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/internal/hive/sessionapi"
)

// BuildInfo holds build-time metadata set by the main package.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// Multiplexer exposes the shared operations used by application commands.
type Multiplexer interface {
	sessionsvc.Multiplexer
	terminal.PaneSource
	ResolveTarget(ctx context.Context, raw string) (multiplexer.Pane, error)
	SendLiteral(ctx context.Context, target multiplexer.Target, text string) error
	SendKey(ctx context.Context, target multiplexer.Target, key multiplexer.NamedKey) error
	Paste(ctx context.Context, target multiplexer.Target, text []byte, opts multiplexer.PasteOptions) error
}

// App is the central entry point for all hive operations. It embeds the
// engine, whose accessors (Sessions, Messages, HC, ...) commands and the TUI
// call per use, and adds what only the CLI has.
type App struct {
	*hive.Engine

	// Logger carries no cmp label. A command or component adds its own.
	Logger zerolog.Logger

	// Config is the CLI config. It shadows Engine.Config, and its embedded
	// engine half is the config the engine was built from.
	Config *config.Config
	// ConfigPath is the config file the CLI loaded, which a daemon it starts
	// must load too.
	ConfigPath  string
	Doctor      *doctor.Service
	Multiplexer Multiplexer
	Plugins     *plugins.Manager
	CommandSet  *plugins.CommandSet
	Build       BuildInfo
	Sources     *sources.Registry
	// Servers is the local session API and each server under servers:.
	Servers *servers.Fleet

	sessions     sessionapi.API
	targetServer string
}

// NewApp wraps an engine built from &cfg.Config with the CLI pieces. Doctor
// validates the whole CLI config, so `hive doctor` keeps reporting errors in
// sections only the CLI reads.
func NewApp(
	logger zerolog.Logger,
	engine *hive.Engine,
	cfg *config.Config,
	multiplexer Multiplexer,
	pluginMgr *plugins.Manager,
	commandSet *plugins.CommandSet,
	pluginInfos []doctor.PluginInfo,
	configPath string,
) *App {
	a := &App{
		Engine:      engine,
		Logger:      logger,
		Config:      cfg,
		ConfigPath:  configPath,
		Doctor:      engine.Doctor(cfg, pluginInfos),
		Multiplexer: multiplexer,
		Plugins:     pluginMgr,
		CommandSet:  commandSet,
	}
	if cfg.Experimental.Daemon {
		a.sessions = a.DaemonClient(true)
	} else {
		a.sessions = sessionapi.NewLocal(engine)
	}
	a.Servers = servers.New(logger, a.sessions, cfg.Servers)
	return a
}

// SessionAPI is the path for session writes, session reads, and session-view
// status: the daemon when experimental.daemon is on, else this process's
// engine. Tmux-only helpers and git reads stay on Sessions().
func (a *App) SessionAPI() sessionapi.API {
	return a.sessions
}

// TargetServer points SessionAPI at the daemon on a server under servers:,
// so session commands act on that server's sessions instead of local ones.
func (a *App) TargetServer(name string) error {
	if name == session.LocalServer {
		return fmt.Errorf("--target-server %q names this machine; omit the flag", name)
	}
	api, err := a.ServerAPI(name)
	if err != nil {
		return err
	}
	a.sessions = api
	a.targetServer = name
	return nil
}

// ServerAPI is the session API of the named server; session.LocalServer
// names this machine. A server's ssh errors go to stderr, since a failed ssh
// otherwise shows only as a closed connection.
func (a *App) ServerAPI(name string) (sessionapi.API, error) {
	if name == session.LocalServer {
		if local := a.Servers.Local(); local != nil {
			return local, nil
		}
		return nil, errors.New("this command has no local session API")
	}
	server, ok := a.Servers.Server(name)
	if !ok {
		return nil, fmt.Errorf("unknown server %q: it is not under servers: in config.yaml", name)
	}
	server.CopyStderrTo(os.Stderr)
	return server.Client(), nil
}

// DefaultServer is the server that SessionAPI reaches and that a bare
// session id names: the --target-server server, else session.LocalServer.
func (a *App) DefaultServer() string {
	if a.targetServer != "" {
		return a.targetServer
	}
	return session.LocalServer
}

// TargetsServer reports whether SessionAPI reaches a server, where paths and
// tmux sessions on this machine mean nothing.
func (a *App) TargetsServer() bool {
	return a.targetServer != ""
}

// DaemonClient reaches the daemon for this data directory. It needs only the
// logger, the config, and ConfigPath, so commands that skip the engine can
// call it. With autoStart, a dial that finds no daemon starts this binary as
// one.
func (a *App) DaemonClient(autoStart bool) *daemonclient.Client {
	opts := daemonclient.Options{
		Dialer: daemonclient.LocalDialer(a.Config.DataDir),
		Logger: a.Logger,
	}
	if autoStart {
		dataDir, configPath := a.Config.DataDir, a.ConfigPath
		opts.AutoStart = func(ctx context.Context) error {
			binary, err := os.Executable()
			if err != nil {
				return fmt.Errorf("find the hive binary: %w", err)
			}
			env := os.Environ()
			if configPath != "" {
				env = append(env, hiveconfig.EnvConfig+"="+configPath)
			}
			return daemonclient.Spawn(ctx, binary, dataDir, env)
		}
	}
	return daemonclient.New(opts)
}
