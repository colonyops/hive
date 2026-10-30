package commands

import (
	"context"
	"fmt"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/cmd/hive/internal/app"
	hiveserver "github.com/colonyops/hive/internal/domain/server"
	"github.com/colonyops/hive/internal/hive/daemon/wire"
	"github.com/colonyops/hive/internal/hive/servers"
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
	State       string `json:"state"`
	Reachable   bool   `json:"reachable"`
	Version     string `json:"version,omitempty"`
	Sessions    int    `json:"sessions"`
	RoundTripMS int64  `json:"round_trip_ms"`
	Error       string `json:"error,omitempty"`
}

func (cmd *ServersCmd) ls(ctx context.Context, c *cli.Command) error {
	all := cmd.app.Servers.Servers()
	results := make([]serverSummary, len(all))
	var wg sync.WaitGroup
	for i, server := range all {
		wg.Go(func() {
			ctx, cancel := context.WithTimeout(ctx, serverProbeTimeout)
			defer cancel()
			results[i] = summarize(ctx, server)
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
	_, _ = fmt.Fprintln(tw, "NAME\tSSH\tSTATE\tVERSION\tSESSIONS\tRTT\tERROR")
	for _, r := range results {
		if !r.Reachable {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t-\t-\t-\t%s\n", r.Name, r.SSH, r.State, r.Error)
			continue
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%dms\t\n", r.Name, r.SSH, r.State, r.Version, r.Sessions, r.RoundTripMS)
	}
	return tw.Flush()
}

func summarize(ctx context.Context, server *servers.Server) serverSummary {
	status := server.Check(ctx)
	out := serverSummary{
		Name:        status.Name,
		SSH:         status.SSH,
		State:       string(status.State),
		Reachable:   status.State == hiveserver.StateConnected,
		Version:     status.Version,
		RoundTripMS: status.RoundTrip.Milliseconds(),
		Error:       status.Error,
	}
	if !out.Reachable {
		return out
	}
	sessions, err := server.Client().ListSessions(ctx)
	if err != nil {
		out.Error = err.Error()
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
		Version  string `json:"version,omitempty"`
		Protocol int    `json:"protocol"`
		PID      int    `json:"pid,omitempty"`
		DataDir  string `json:"data_dir,omitempty"`
	} `json:"hello"`
	Health struct {
		probeStep
		DBOK       bool           `json:"db_ok"`
		TmuxOK     bool           `json:"tmux_ok"`
		ByState    map[string]int `json:"sessions_by_state,omitempty"`
		UptimeMS   int64          `json:"uptime_ms"`
		Supervised bool           `json:"supervised"`
		Agents     []probeAgent   `json:"agents"`
	} `json:"health"`
	List     probeStep      `json:"list"`
	Sessions []probeSession `json:"sessions"`
}

type probeAgent struct {
	Profile string `json:"profile"`
	Command string `json:"command"`
	// Path is empty when the command is not on the daemon's PATH.
	Path string `json:"path"`
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
	server, ok := cmd.app.Servers.Server(name)
	if !ok {
		return fmt.Errorf("unknown server %q: it is not under servers: in config.yaml", name)
	}
	ctx, cancel := context.WithTimeout(ctx, serverProbeTimeout)
	defer cancel()
	client := server.Client()

	out := probeResult{Name: name, SSH: server.Config().SSH, Sessions: []probeSession{}}
	failed := func(step *probeStep, err error) error {
		step.Error = err.Error()
		if cmd.json {
			_ = iojson.WriteLine(c.Root().Writer, out)
		}
		return fmt.Errorf("probe %s: %w", name, err)
	}

	began := time.Now()
	info, err := client.Hello(ctx)
	out.Hello.MS = time.Since(began).Milliseconds()
	if err != nil {
		return failed(&out.Hello.probeStep, err)
	}
	out.Hello.Version, out.Hello.Protocol, out.Hello.PID, out.Hello.DataDir = info.Version, info.ProtocolVersion, info.PID, info.DataDir
	if err := wire.CheckProtocol(info.ProtocolVersion, info.Version, cmd.app.Build.Version); err != nil {
		return failed(&out.Hello.probeStep, err)
	}

	began = time.Now()
	health, err := client.Health(ctx)
	out.Health.MS = time.Since(began).Milliseconds()
	if err != nil {
		return failed(&out.Health.probeStep, err)
	}
	out.Health.DBOK, out.Health.TmuxOK = health.DBOK, health.TmuxOK
	out.Health.ByState, out.Health.UptimeMS = health.SessionsByState, health.Uptime.Milliseconds()
	out.Health.Supervised = health.Supervised
	out.Health.Agents = []probeAgent{}
	for _, agent := range health.Agents {
		out.Health.Agents = append(out.Health.Agents, probeAgent(agent))
	}

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
	// A daemon on protocol 0 does not report this.
	if !health.Supervised && info.ProtocolVersion > 0 {
		_, _ = fmt.Fprintln(w, "daemon         not started by `hive daemon install`; it does not start again after the server restarts")
	}
	for _, agent := range out.Health.Agents {
		if agent.Path == "" {
			_, _ = fmt.Fprintf(w, "agent          %s: %s is not on the daemon's PATH\n", agent.Profile, agent.Command)
		}
	}
	_, _ = fmt.Fprintf(w, "list    %4dms  %d sessions\n", out.List.MS, len(out.Sessions))
	for _, sess := range out.Sessions {
		_, _ = fmt.Fprintf(w, "  %s  %s  %s\n", sess.ID, sess.State, sess.Name)
	}
	return nil
}
