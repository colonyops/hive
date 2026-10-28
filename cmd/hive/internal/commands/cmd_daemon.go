package commands

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/cmd/hive/internal/app"
	"github.com/colonyops/hive/cmd/hive/internal/config"
	engineconfig "github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/hive/daemon"
	"github.com/colonyops/hive/internal/hive/daemonclient"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/internal/platform/credentials"
	"github.com/colonyops/hive/internal/platform/execenv"
	tmuxexec "github.com/colonyops/hive/internal/platform/tmux/exec"
	"github.com/colonyops/hive/pkg/executil"
	"github.com/colonyops/hive/pkg/iojson"
)

type DaemonCmd struct {
	flags *Flags
	app   *app.App

	waitLock      bool
	statusJSON    bool
	statusNoStart bool
}

func NewDaemonCmd(flags *Flags, app *app.App) *DaemonCmd {
	return &DaemonCmd{flags: flags, app: app}
}

func (cmd *DaemonCmd) Register(root *cli.Command) *cli.Command {
	root.Commands = append(root.Commands, &cli.Command{
		Name:  "daemon",
		Usage: "Run or inspect the hive daemon (experimental)",
		Description: `The hive daemon owns session writes and agent status for one data
directory. With experimental.daemon set in config.yaml, the TUI, the session
commands, and Hive Desktop reach it on a unix socket.`,
		Commands: []*cli.Command{
			{
				Name:  "run",
				Usage: "Run the daemon in the foreground",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:        "wait-lock",
						Usage:       "wait for a running daemon to exit instead of failing",
						Destination: &cmd.waitLock,
					},
				},
				Action: cmd.run,
			},
			{
				Name:  "status",
				Usage: "Show the running daemon's pid, version, uptime, and health",
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "json", Usage: "output as JSON", Destination: &cmd.statusJSON},
					&cli.BoolFlag{Name: "no-start", Usage: "fail instead of starting a daemon when none is running", Destination: &cmd.statusNoStart},
				},
				Action: cmd.status,
			},
			{
				Name:   "stop",
				Usage:  "Stop the running daemon",
				Action: cmd.stop,
			},
		},
	})
	return root
}

func (cmd *DaemonCmd) run(ctx context.Context, _ *cli.Command) error {
	logger := cmd.app.Logger
	// launchd and systemd start the daemon with a bare PATH, and agents it
	// spawns need the user's tools.
	resolver := execenv.NewResolver(execenv.Options{Logger: logger})
	if path := resolver.Path(ctx); path != "" {
		if err := os.Setenv("PATH", path); err != nil {
			return fmt.Errorf("set PATH: %w", err)
		}
	}

	tmuxClient := tmuxexec.NewDefault(logger)
	err := daemon.Run(ctx, daemon.Options{
		Config: &cmd.app.Config.Config,
		Ports: hive.Ports{
			Executor:   &executil.RealExecutor{},
			Mux:        tmuxClient,
			PaneSource: tmuxClient,
			DataDir:    cmd.flags.DataDir,
			Styler:     sessionsvc.PlainStyler{},
			Stdout:     newLogWriter(logger, zerolog.InfoLevel),
			Stderr:     newLogWriter(logger, zerolog.WarnLevel),
			Logger:     logger,
			Credentials: credentials.NewKeychainStoreWithService(
				filepath.Join(cmd.flags.DataDir, "drive", "credentials.json"),
				os.Getenv(credentials.EnvKeyringService),
			),
		},
		Version:  cmd.app.Build.Version,
		WaitLock: cmd.waitLock,
		Reload: func(context.Context) (*engineconfig.Config, error) {
			cfg, err := config.Load(cmd.flags.ConfigPath, cmd.flags.DataDir)
			if err != nil {
				return nil, err
			}
			return &cfg.Config, nil
		},
	})
	if errors.Is(err, daemon.ErrAlreadyRunning) {
		return fmt.Errorf("%w (socket %s)", err, daemon.SocketPath(cmd.flags.DataDir))
	}
	return err
}

type daemonStatus struct {
	PID             int            `json:"pid"`
	Version         string         `json:"version"`
	DataDir         string         `json:"data_dir"`
	StartedAt       time.Time      `json:"started_at"`
	UptimeSeconds   float64        `json:"uptime_seconds"`
	StatusAvailable bool           `json:"status_available"`
	DBOK            bool           `json:"db_ok"`
	DBError         string         `json:"db_error,omitempty"`
	TmuxOK          bool           `json:"tmux_ok"`
	Sessions        map[string]int `json:"sessions"`
	StatusLoop      struct {
		LastTick       time.Time `json:"last_tick,omitzero"`
		LastDurationMS int64     `json:"last_duration_ms"`
		Errors         int64     `json:"errors"`
		Paused         bool      `json:"paused"`
	} `json:"status_loop"`
}

func (cmd *DaemonCmd) status(ctx context.Context, c *cli.Command) error {
	client := cmd.app.DaemonClient(!cmd.statusNoStart)
	info, err := client.Hello(ctx)
	if err != nil {
		return fmt.Errorf("reach the daemon at %s: %w", daemon.SocketPath(cmd.flags.DataDir), err)
	}
	health, err := client.Health(ctx)
	if err != nil {
		return fmt.Errorf("read daemon health: %w", err)
	}
	out := buildDaemonStatus(info, health)
	if cmd.statusJSON {
		return iojson.WriteLine(c.Root().Writer, out)
	}
	w := c.Root().Writer
	_, _ = fmt.Fprintf(w, "pid:      %d\n", out.PID)
	_, _ = fmt.Fprintf(w, "version:  %s\n", out.Version)
	_, _ = fmt.Fprintf(w, "uptime:   %s\n", health.Uptime.Round(time.Second))
	_, _ = fmt.Fprintf(w, "data dir: %s\n", out.DataDir)
	_, _ = fmt.Fprintf(w, "database: %s\n", okText(out.DBOK, out.DBError))
	_, _ = fmt.Fprintf(w, "tmux:     %s\n", okText(out.TmuxOK, ""))
	_, _ = fmt.Fprintf(w, "sessions: %d active, %d recycled, %d corrupted\n",
		out.Sessions[string(session.StateActive)], out.Sessions[string(session.StateRecycled)], out.Sessions[string(session.StateCorrupted)])
	loop := health.StatusLoop
	switch {
	case loop.Paused:
		_, _ = fmt.Fprintf(w, "status:   paused (no client asked recently)\n")
	case !loop.LastTick.IsZero():
		_, _ = fmt.Fprintf(w, "status:   last tick %s ago, took %s, %d errors\n",
			time.Since(loop.LastTick).Round(time.Millisecond), loop.LastDuration, loop.Errors)
	}
	return nil
}

func buildDaemonStatus(info daemonclient.Info, health daemonclient.Health) daemonStatus {
	out := daemonStatus{
		PID:             info.PID,
		Version:         info.Version,
		DataDir:         info.DataDir,
		StartedAt:       info.StartedAt,
		UptimeSeconds:   health.Uptime.Seconds(),
		StatusAvailable: info.StatusAvailable,
		DBOK:            health.DBOK,
		DBError:         health.DBError,
		TmuxOK:          health.TmuxOK,
		Sessions:        health.SessionsByState,
	}
	out.StatusLoop.LastTick = health.StatusLoop.LastTick
	out.StatusLoop.LastDurationMS = health.StatusLoop.LastDuration.Milliseconds()
	out.StatusLoop.Errors = health.StatusLoop.Errors
	out.StatusLoop.Paused = health.StatusLoop.Paused
	return out
}

func okText(ok bool, detail string) string {
	switch {
	case ok:
		return "ok"
	case detail != "":
		return "failing: " + detail
	default:
		return "unavailable"
	}
}

func (cmd *DaemonCmd) stop(ctx context.Context, _ *cli.Command) error {
	if err := cmd.app.DaemonClient(false).Stop(ctx); err != nil {
		return fmt.Errorf("stop the daemon: %w", err)
	}
	fmt.Fprintln(os.Stderr, "daemon stopped")
	return nil
}

// newLogWriter turns the session service's terminal output into log lines:
// the daemon has no terminal, and its progress goes to the client per call.
func newLogWriter(logger zerolog.Logger, level zerolog.Level) io.Writer {
	reader, writer := io.Pipe()
	go func() {
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			logger.WithLevel(level).Str("stream", "session-service").Msg(scanner.Text())
		}
	}()
	return writer
}
