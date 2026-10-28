package commands

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
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
			{
				Name:   "install",
				Usage:  "Run the daemon under launchd (macOS) or systemd (Linux) so it restarts on exit",
				Action: cmd.install,
			},
			{
				Name:   "uninstall",
				Usage:  "Remove the launchd agent or systemd unit that install wrote",
				Action: cmd.uninstall,
			},
			{
				Name:   "dial-stdio",
				Usage:  "Bridge stdin and stdout to the daemon socket, starting the daemon if needed",
				Hidden: true,
				Action: cmd.dialStdio,
			},
			{
				Name:   "metrics",
				Usage:  "Print the running daemon's Prometheus metrics",
				Action: cmd.metrics,
			},
		},
	})
	return root
}

func (cmd *DaemonCmd) run(ctx context.Context, _ *cli.Command) error {
	ctx, stopSignals := signal.NotifyContext(ctx, syscall.SIGTERM, syscall.SIGINT)
	defer stopSignals()
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

// dialStdio is the remote end of `ssh <host> -- hive daemon dial-stdio`.
func (cmd *DaemonCmd) dialStdio(ctx context.Context, _ *cli.Command) error {
	conn, err := cmd.app.DaemonClient(true).Dial(ctx)
	if err != nil {
		return fmt.Errorf("reach the daemon at %s: %w", daemon.SocketPath(cmd.flags.DataDir), err)
	}
	defer func() { _ = conn.Close() }()

	go func() {
		_, _ = io.Copy(conn, os.Stdin)
		// Half-close so the daemon sees EOF and the reply still drains.
		if cw, ok := conn.(interface{ CloseWrite() error }); ok {
			_ = cw.CloseWrite()
		}
	}()
	_, err = io.Copy(os.Stdout, conn)
	return err
}

func (cmd *DaemonCmd) metrics(ctx context.Context, c *cli.Command) error {
	body, err := cmd.app.DaemonClient(false).Metrics(ctx)
	if err != nil {
		return fmt.Errorf("read daemon metrics: %w", err)
	}
	_, err = c.Root().Writer.Write(body)
	return err
}

func (cmd *DaemonCmd) serviceUnit() (daemon.Unit, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return daemon.Unit{}, err
	}
	binary, err := os.Executable()
	if err != nil {
		return daemon.Unit{}, fmt.Errorf("find the hive binary: %w", err)
	}
	dataDir, err := filepath.Abs(cmd.flags.DataDir)
	if err != nil {
		return daemon.Unit{}, err
	}
	configPath, err := filepath.Abs(cmd.flags.ConfigPath)
	if err != nil {
		return daemon.Unit{}, err
	}
	switch runtime.GOOS {
	case "darwin":
		return daemon.LaunchdUnit(home, binary, dataDir, configPath), nil
	case "linux":
		return daemon.SystemdUnit(home, binary, dataDir, configPath), nil
	default:
		return daemon.Unit{}, fmt.Errorf("hive daemon install supports macOS and Linux, not %s", runtime.GOOS)
	}
}

func (cmd *DaemonCmd) install(ctx context.Context, c *cli.Command) error {
	unit, err := cmd.serviceUnit()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(unit.Path), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(unit.Path), err)
	}
	// launchd opens the output log itself and fails when its directory is
	// missing.
	if err := os.MkdirAll(filepath.Dir(daemon.OutLogPath(cmd.flags.DataDir)), 0o755); err != nil {
		return fmt.Errorf("create daemon log directory: %w", err)
	}
	if err := os.WriteFile(unit.Path, []byte(unit.Contents), 0o644); err != nil { //nolint:gosec // launchd and systemd read unit files the user owns
		return fmt.Errorf("write %s: %w", unit.Path, err)
	}

	// A daemon a client spawned keeps the lock, so the supervised one would
	// wait behind it until it exits.
	if err := cmd.app.DaemonClient(false).Stop(ctx); err == nil {
		fmt.Fprintln(os.Stderr, "stopped the running daemon so the service can take over")
	}

	w := c.Root().Writer
	switch runtime.GOOS {
	case "darwin":
		domain := "gui/" + strconv.Itoa(os.Getuid())
		// bootstrap fails on a loaded agent; bootout first makes install
		// repeatable.
		_ = exec.CommandContext(ctx, "launchctl", "bootout", domain+"/"+daemon.LaunchdLabel).Run()
		if err := runService(ctx, "launchctl", "bootstrap", domain, unit.Path); err != nil {
			return err
		}
	default:
		if err := runService(ctx, "systemctl", "--user", "daemon-reload"); err != nil {
			return err
		}
		if err := runService(ctx, "systemctl", "--user", "enable", "--now", daemon.SystemdUnitName); err != nil {
			return err
		}
	}
	_, _ = fmt.Fprintf(w, "installed %s\n", unit.Path)
	if runtime.GOOS == "linux" {
		_, _ = fmt.Fprintln(w, "To keep the daemon running after you log out, run: loginctl enable-linger $USER")
	}
	return nil
}

func (cmd *DaemonCmd) uninstall(ctx context.Context, c *cli.Command) error {
	unit, err := cmd.serviceUnit()
	if err != nil {
		return err
	}
	switch runtime.GOOS {
	case "darwin":
		target := "gui/" + strconv.Itoa(os.Getuid()) + "/" + daemon.LaunchdLabel
		if err := runService(ctx, "launchctl", "bootout", target); err != nil {
			fmt.Fprintf(os.Stderr, "warning: %v\n", err)
		}
	default:
		if err := runService(ctx, "systemctl", "--user", "disable", "--now", daemon.SystemdUnitName); err != nil {
			fmt.Fprintf(os.Stderr, "warning: %v\n", err)
		}
	}
	if err := os.Remove(unit.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", unit.Path, err)
	}
	if runtime.GOOS == "linux" {
		if err := runService(ctx, "systemctl", "--user", "daemon-reload"); err != nil {
			fmt.Fprintf(os.Stderr, "warning: %v\n", err)
		}
	}
	_, _ = fmt.Fprintf(c.Root().Writer, "removed %s\n", unit.Path)
	return nil
}

func runService(ctx context.Context, name string, args ...string) error {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
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
