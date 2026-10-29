package daemon

import (
	"context"
	"errors"
	"maps"
	"os"
	"os/exec"
	"slices"
	"time"

	"connectrpc.com/connect"
	"github.com/rs/zerolog"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/terminal"
	"github.com/colonyops/hive/internal/hive"
	daemonv1 "github.com/colonyops/hive/internal/hive/daemon/gen/hive/daemon/v1"
	"github.com/colonyops/hive/internal/hive/daemon/gen/hive/daemon/v1/daemonv1connect"
	"github.com/colonyops/hive/internal/hive/daemon/wire"
	"github.com/colonyops/hive/internal/hive/sessionapi"
)

type controlServer struct {
	runtime   *hive.Runtime
	api       sessionapi.API
	paneOK    terminal.PaneSource
	version   string
	dataDir   string
	startedAt time.Time
	reload    func(ctx context.Context) (*config.Config, error)
	stop      context.CancelFunc
	logger    zerolog.Logger
	loop      func() *daemonv1.StatusLoopHealth
	// supervised marks a daemon that an OS service unit started.
	supervised bool
}

var _ daemonv1connect.DaemonServiceHandler = (*controlServer)(nil)

func (c *controlServer) Hello(context.Context, *connect.Request[daemonv1.HelloRequest]) (*connect.Response[daemonv1.HelloResponse], error) {
	return connect.NewResponse(&daemonv1.HelloResponse{
		Version:         c.version,
		Pid:             int32(os.Getpid()), //nolint:gosec // a pid fits in int32
		DataDir:         c.dataDir,
		StartedAt:       timestamppb.New(c.startedAt),
		StatusAvailable: c.api.StatusAvailable(),
		ProtocolVersion: wire.ProtocolVersion,
	}), nil
}

func (c *controlServer) Health(ctx context.Context, _ *connect.Request[daemonv1.HealthRequest]) (*connect.Response[daemonv1.HealthResponse], error) {
	resp := &daemonv1.HealthResponse{
		DbOk:            true,
		SessionsByState: map[string]int32{},
		Uptime:          durationpb.New(time.Since(c.startedAt)),
	}
	if err := c.runtime.DB().Conn().PingContext(ctx); err != nil {
		resp.DbOk = false
		resp.DbError = err.Error()
	}
	if c.paneOK != nil {
		_, err := c.paneOK.ListPanes(ctx)
		resp.TmuxOk = err == nil
	}
	if sessions, err := c.api.ListSessions(ctx); err == nil {
		for _, sess := range sessions {
			resp.SessionsByState[string(sess.State)]++
		}
	} else {
		resp.DbOk = false
		resp.DbError = err.Error()
	}
	if c.loop != nil {
		resp.StatusLoop = c.loop()
	}
	resp.Supervised = c.supervised
	resp.Agents = agentChecks(c.runtime.Engine().Config().Agents)
	return connect.NewResponse(resp), nil
}

// agentChecks looks up each profile's command on the daemon's PATH, which is
// the PATH its sessions start agents with.
func agentChecks(agents config.AgentsConfig) []*daemonv1.AgentCheck {
	names := slices.Sorted(maps.Keys(agents.Profiles))
	checks := make([]*daemonv1.AgentCheck, 0, len(names))
	for _, name := range names {
		command := agents.Profiles[name].CommandOrDefault(name)
		check := &daemonv1.AgentCheck{Profile: name, Command: command}
		if path, err := exec.LookPath(command); err == nil {
			check.Path = path
		}
		checks = append(checks, check)
	}
	return checks
}

func (c *controlServer) Stop(context.Context, *connect.Request[daemonv1.StopRequest]) (*connect.Response[daemonv1.StopResponse], error) {
	c.logger.Info().Msg("stop requested")
	// The server's graceful shutdown waits for this response to go out.
	c.stop()
	return connect.NewResponse(&daemonv1.StopResponse{}), nil
}

func (c *controlServer) Reload(ctx context.Context, _ *connect.Request[daemonv1.ReloadRequest]) (*connect.Response[daemonv1.ReloadResponse], error) {
	if c.reload == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("this daemon cannot reload its config"))
	}
	cfg, err := c.reload(ctx)
	if err != nil {
		return nil, wire.ToConnectError(err)
	}
	if err := c.runtime.Engine().Reload(cfg); err != nil {
		return nil, wire.ToConnectError(err)
	}
	return connect.NewResponse(&daemonv1.ReloadResponse{}), nil
}
