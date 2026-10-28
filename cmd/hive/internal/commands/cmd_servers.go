package commands

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/rs/zerolog"
	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/cmd/hive/internal/app"
	"github.com/colonyops/hive/cmd/hive/internal/config"
	"github.com/colonyops/hive/internal/hive/daemonclient"
	"github.com/colonyops/hive/internal/platform/sshdial"
	"github.com/colonyops/hive/pkg/iojson"
)

const serverProbeTimeout = 10 * time.Second

type ServersCmd struct {
	flags *Flags
	app   *app.App

	json bool
}

func NewServersCmd(flags *Flags, app *app.App) *ServersCmd {
	return &ServersCmd{flags: flags, app: app}
}

func (cmd *ServersCmd) Register(root *cli.Command) *cli.Command {
	jsonFlag := &cli.BoolFlag{Name: "json", Usage: "output as JSON", Destination: &cmd.json}
	root.Commands = append(root.Commands, &cli.Command{
		Name:  "servers",
		Usage: "Reach remote hive daemons over ssh (experimental)",
		Description: `Servers are listed under servers: in config.yaml. Each is an ssh host that
runs "hive daemon dial-stdio", which needs no daemon flag on either side.`,
		Commands: []*cli.Command{
			{
				Name:   "ls",
				Usage:  "Probe every configured server and show which ones answer",
				Flags:  []cli.Flag{jsonFlag},
				Action: cmd.ls,
			},
			{
				Name:      "probe",
				Usage:     "Run Hello, Health, and List against one server and time each step",
				ArgsUsage: "<name>",
				Flags:     []cli.Flag{jsonFlag},
				Action:    cmd.probe,
			},
		},
	})
	return root
}

type serverSummary struct {
	Name        string `json:"name"`
	SSH         string `json:"ssh"`
	Reachable   bool   `json:"reachable"`
	Version     string `json:"version,omitempty"`
	Sessions    int    `json:"sessions"`
	RoundTripMS int64  `json:"round_trip_ms"`
	Error       string `json:"error,omitempty"`
}

func (cmd *ServersCmd) ls(ctx context.Context, c *cli.Command) error {
	servers := cmd.app.Config.Servers
	results := make([]serverSummary, len(servers))
	var wg sync.WaitGroup
	for i, server := range servers {
		wg.Go(func() {
			ctx, cancel := context.WithTimeout(ctx, serverProbeTimeout)
			defer cancel()
			results[i] = summarize(ctx, cmd.app.Logger, server)
		})
	}
	wg.Wait()

	w := c.Root().Writer
	if cmd.json {
		for _, r := range results {
			if err := iojson.WriteLine(w, r); err != nil {
				return err
			}
		}
		return nil
	}
	if len(results) == 0 {
		_, _ = fmt.Fprintln(w, "no servers configured; add them under servers: in config.yaml")
		return nil
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tSSH\tREACHABLE\tVERSION\tSESSIONS\tRTT\tERROR")
	for _, r := range results {
		if !r.Reachable {
			_, _ = fmt.Fprintf(tw, "%s\t%s\tno\t-\t-\t-\t%s\n", r.Name, r.SSH, r.Error)
			continue
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\tyes\t%s\t%d\t%dms\t\n", r.Name, r.SSH, r.Version, r.Sessions, r.RoundTripMS)
	}
	return tw.Flush()
}

func summarize(ctx context.Context, logger zerolog.Logger, server config.ServerConfig) serverSummary {
	out := serverSummary{Name: server.Name, SSH: server.SSH}
	client, stderr := remoteClient(logger, server)
	defer client.HTTPClient().CloseIdleConnections()

	began := time.Now()
	info, err := client.Hello(ctx)
	out.RoundTripMS = time.Since(began).Milliseconds()
	if err != nil {
		out.Error = remoteError(err, stderr).Error()
		return out
	}
	out.Reachable = true
	out.Version = info.Version
	sessions, err := client.ListSessions(ctx)
	if err != nil {
		out.Error = remoteError(err, stderr).Error()
		return out
	}
	out.Sessions = len(sessions)
	return out
}

type probeStep struct {
	MS    int64  `json:"ms"`
	Error string `json:"error,omitempty"`
}

type probeResult struct {
	Name  string `json:"name"`
	SSH   string `json:"ssh"`
	Hello struct {
		probeStep
		Version string `json:"version,omitempty"`
		PID     int    `json:"pid,omitempty"`
		DataDir string `json:"data_dir,omitempty"`
	} `json:"hello"`
	Health struct {
		probeStep
		DBOK     bool           `json:"db_ok"`
		TmuxOK   bool           `json:"tmux_ok"`
		ByState  map[string]int `json:"sessions_by_state,omitempty"`
		UptimeMS int64          `json:"uptime_ms"`
	} `json:"health"`
	List     probeStep      `json:"list"`
	Sessions []probeSession `json:"sessions"`
}

type probeSession struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
}

func (cmd *ServersCmd) probe(ctx context.Context, c *cli.Command) error {
	name := c.Args().First()
	if name == "" {
		return fmt.Errorf("usage: hive servers probe <name>")
	}
	server, ok := cmd.app.Config.Server(name)
	if !ok {
		return fmt.Errorf("no server named %q under servers: in config.yaml", name)
	}
	ctx, cancel := context.WithTimeout(ctx, serverProbeTimeout)
	defer cancel()
	client, stderr := remoteClient(cmd.app.Logger, server)
	defer client.HTTPClient().CloseIdleConnections()

	out := probeResult{Name: server.Name, SSH: server.SSH, Sessions: []probeSession{}}
	failed := func(step *probeStep, err error) error {
		err = remoteError(err, stderr)
		step.Error = err.Error()
		if cmd.json {
			_ = iojson.WriteLine(c.Root().Writer, out)
		}
		return fmt.Errorf("probe %s: %w", server.Name, err)
	}

	began := time.Now()
	info, err := client.Hello(ctx)
	out.Hello.MS = time.Since(began).Milliseconds()
	if err != nil {
		return failed(&out.Hello.probeStep, err)
	}
	out.Hello.Version, out.Hello.PID, out.Hello.DataDir = info.Version, info.PID, info.DataDir

	began = time.Now()
	health, err := client.Health(ctx)
	out.Health.MS = time.Since(began).Milliseconds()
	if err != nil {
		return failed(&out.Health.probeStep, err)
	}
	out.Health.DBOK, out.Health.TmuxOK = health.DBOK, health.TmuxOK
	out.Health.ByState, out.Health.UptimeMS = health.SessionsByState, health.Uptime.Milliseconds()

	began = time.Now()
	sessions, err := client.ListSessions(ctx)
	out.List.MS = time.Since(began).Milliseconds()
	if err != nil {
		return failed(&out.List, err)
	}
	for _, sess := range sessions {
		out.Sessions = append(out.Sessions, probeSession{ID: sess.ID, Name: sess.Name, State: string(sess.State)})
	}

	w := c.Root().Writer
	if cmd.json {
		return iojson.WriteLine(w, out)
	}
	_, _ = fmt.Fprintf(w, "hello   %4dms  version %s, pid %d, data dir %s\n", out.Hello.MS, out.Hello.Version, out.Hello.PID, out.Hello.DataDir)
	_, _ = fmt.Fprintf(w, "health  %4dms  database %s, tmux %s, up %s\n", out.Health.MS, okText(health.DBOK, health.DBError), okText(health.TmuxOK, ""), health.Uptime.Round(time.Second))
	_, _ = fmt.Fprintf(w, "list    %4dms  %d sessions\n", out.List.MS, len(out.Sessions))
	for _, sess := range out.Sessions {
		_, _ = fmt.Fprintf(w, "  %s  %s  %s\n", sess.ID, sess.State, sess.Name)
	}
	return nil
}

// remoteClient never auto-starts: dial-stdio on the remote side starts the
// daemon there.
func remoteClient(logger zerolog.Logger, server config.ServerConfig) (*daemonclient.Client, *tailBuffer) {
	stderr := &tailBuffer{}
	host := server.Host()
	client := daemonclient.New(daemonclient.Options{
		Logger: logger,
		Dialer: func(ctx context.Context) (net.Conn, error) {
			return sshdial.DialStdio(ctx, host, stderr)
		},
	})
	return client, stderr
}

// remoteError adds ssh's last stderr line, since a failed ssh only shows up
// as a closed connection.
func remoteError(err error, stderr *tailBuffer) error {
	if line := stderr.lastLine(); line != "" {
		return fmt.Errorf("%w; %s", err, line)
	}
	return err
}

// tailBuffer keeps the last 4 KiB written to it.
type tailBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

var _ io.Writer = (*tailBuffer)(nil)

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.Write(p)
	if over := b.buf.Len() - 4096; over > 0 {
		b.buf.Next(over)
	}
	return len(p), nil
}

func (b *tailBuffer) lastLine() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	lines := strings.Split(strings.TrimSpace(b.buf.String()), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
