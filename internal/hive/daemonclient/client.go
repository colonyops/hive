// Package daemonclient implements sessionapi.API against a hive daemon.
package daemonclient

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"connectrpc.com/connect"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/domain/multiplexer"
	"github.com/colonyops/hive/internal/domain/session"
	daemonv1 "github.com/colonyops/hive/internal/hive/daemon/gen/hive/daemon/v1"
	"github.com/colonyops/hive/internal/hive/daemon/gen/hive/daemon/v1/daemonv1connect"
	"github.com/colonyops/hive/internal/hive/daemon/wire"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/internal/hive/sessionapi"
	"github.com/colonyops/hive/internal/hive/status"
	"github.com/colonyops/hive/pkg/logutils"
)

// Dialer opens one connection to a daemon.
type Dialer func(ctx context.Context) (net.Conn, error)

func UnixDialer(path string) Dialer {
	return func(ctx context.Context) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "unix", path)
	}
}

type Options struct {
	Dialer Dialer
	Logger zerolog.Logger
	// AutoStart starts a daemon when a dial finds none. nil never starts one,
	// as for a remote daemon or `hive daemon status --no-start`.
	AutoStart func(ctx context.Context) error
}

// baseURL is never resolved: every connection goes through the Dialer.
const baseURL = "http://hived"

// statusTimeout bounds FetchBatch, which has no error to return.
const statusTimeout = 5 * time.Second

type Client struct {
	logger     zerolog.Logger
	dialer     Dialer
	starter    *starter
	httpClient *http.Client
	sessions   daemonv1connect.SessionServiceClient
	control    daemonv1connect.DaemonServiceClient

	statusAvailable atomic.Bool
	helloOnce       sync.Once
}

var _ sessionapi.API = (*Client)(nil)

func New(opts Options) *Client {
	c := &Client{
		logger: logutils.Component(opts.Logger, "daemon-client"),
		dialer: opts.Dialer,
	}
	if opts.AutoStart != nil {
		c.starter = &starter{start: opts.AutoStart, now: time.Now}
	}
	var protocols http.Protocols
	protocols.SetUnencryptedHTTP2(true)
	c.httpClient = &http.Client{Transport: &http.Transport{
		Protocols: &protocols,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return c.dial(ctx)
		},
	}}
	c.sessions = daemonv1connect.NewSessionServiceClient(c.httpClient, baseURL)
	c.control = daemonv1connect.NewDaemonServiceClient(c.httpClient, baseURL)
	return c
}

// HTTPClient reaches the daemon's plain HTTP endpoints, such as /healthz.
func (c *Client) HTTPClient() *http.Client { return c.httpClient }

type Info struct {
	Version         string
	PID             int
	DataDir         string
	StartedAt       time.Time
	StatusAvailable bool
}

type StatusLoopHealth struct {
	LastTick     time.Time
	LastDuration time.Duration
	Errors       int64
	Paused       bool
}

type Health struct {
	DBOK            bool
	DBError         string
	TmuxOK          bool
	StatusLoop      StatusLoopHealth
	SessionsByState map[string]int
	Uptime          time.Duration
}

// Hello also refreshes the cached answer StatusAvailable gives.
func (c *Client) Hello(ctx context.Context) (Info, error) {
	resp, err := retryRead(func() (*connect.Response[daemonv1.HelloResponse], error) {
		return c.control.Hello(ctx, connect.NewRequest(&daemonv1.HelloRequest{}))
	})
	if err != nil {
		return Info{}, wire.FromConnectError(err)
	}
	msg := resp.Msg
	c.statusAvailable.Store(msg.GetStatusAvailable())
	return Info{
		Version:         msg.GetVersion(),
		PID:             int(msg.GetPid()),
		DataDir:         msg.GetDataDir(),
		StartedAt:       msg.GetStartedAt().AsTime().Local(),
		StatusAvailable: msg.GetStatusAvailable(),
	}, nil
}

func (c *Client) Health(ctx context.Context) (Health, error) {
	resp, err := retryRead(func() (*connect.Response[daemonv1.HealthResponse], error) {
		return c.control.Health(ctx, connect.NewRequest(&daemonv1.HealthRequest{}))
	})
	if err != nil {
		return Health{}, wire.FromConnectError(err)
	}
	msg := resp.Msg
	health := Health{
		DBOK:            msg.GetDbOk(),
		DBError:         msg.GetDbError(),
		TmuxOK:          msg.GetTmuxOk(),
		SessionsByState: map[string]int{},
		Uptime:          msg.GetUptime().AsDuration(),
	}
	for state, n := range msg.GetSessionsByState() {
		health.SessionsByState[state] = int(n)
	}
	if loop := msg.GetStatusLoop(); loop != nil {
		health.StatusLoop = StatusLoopHealth{
			LastDuration: time.Duration(loop.GetLastDurationMs()) * time.Millisecond,
			Errors:       loop.GetErrors(),
			Paused:       loop.GetPaused(),
		}
		if loop.GetLastTick() != nil {
			health.StatusLoop.LastTick = loop.GetLastTick().AsTime().Local()
		}
	}
	return health, nil
}

func (c *Client) Stop(ctx context.Context) error {
	_, err := c.control.Stop(ctx, connect.NewRequest(&daemonv1.StopRequest{}))
	return wire.FromConnectError(err)
}

func (c *Client) Reload(ctx context.Context) error {
	_, err := c.control.Reload(ctx, connect.NewRequest(&daemonv1.ReloadRequest{}))
	return wire.FromConnectError(err)
}

func (c *Client) ListSessions(ctx context.Context) ([]session.Session, error) {
	resp, err := retryRead(func() (*connect.Response[daemonv1.ListResponse], error) {
		return c.sessions.List(ctx, connect.NewRequest(&daemonv1.ListRequest{}))
	})
	if err != nil {
		return nil, wire.FromConnectError(err)
	}
	out := make([]session.Session, 0, len(resp.Msg.GetSessions()))
	for _, s := range resp.Msg.GetSessions() {
		out = append(out, wire.SessionFromProto(s))
	}
	return out, nil
}

func (c *Client) GetSession(ctx context.Context, id string) (session.Session, error) {
	resp, err := retryRead(func() (*connect.Response[daemonv1.GetResponse], error) {
		return c.sessions.Get(ctx, connect.NewRequest(&daemonv1.GetRequest{Id: id}))
	})
	if err != nil {
		return session.Session{}, wire.FromConnectError(err)
	}
	return wire.SessionFromProto(resp.Msg.GetSession()), nil
}

// CreateSession always creates in the background, whatever opts.Background
// says: the caller attaches to the returned session itself.
func (c *Client) CreateSession(ctx context.Context, opts sessionsvc.CreateOptions) (*session.Session, error) {
	stream, err := c.sessions.Create(ctx, connect.NewRequest(wire.CreateOptionsToProto(opts)))
	if err != nil {
		return nil, wire.FromConnectError(err)
	}
	defer func() { _ = stream.Close() }()

	var created *session.Session
	for stream.Receive() {
		switch event := stream.Msg().GetEvent().(type) {
		case *daemonv1.CreateResponse_ProgressLine:
			if opts.Progress != nil {
				_, _ = fmt.Fprintln(opts.Progress, event.ProgressLine)
			}
		case *daemonv1.CreateResponse_Session:
			sess := wire.SessionFromProto(event.Session)
			created = &sess
		}
	}
	if err := stream.Err(); err != nil {
		return nil, wire.FromConnectError(err)
	}
	if created == nil {
		return nil, fmt.Errorf("create session: the daemon ended the stream without a session")
	}
	return created, nil
}

func (c *Client) CreateFromRequest(ctx context.Context, req sessionsvc.LaunchRequest) (session.Session, error) {
	resp, err := c.sessions.CreateFromRequest(ctx, connect.NewRequest(wire.LaunchRequestToProto(req)))
	if err != nil {
		return session.Session{}, wire.FromConnectError(err)
	}
	return wire.SessionFromProto(resp.Msg.GetSession()), nil
}

// CreateSessionWithWindows always creates in the background, whatever
// background says: the caller switches to the returned session itself.
func (c *Client) CreateSessionWithWindows(ctx context.Context, req sessionsvc.NewSessionRequest, windows []multiplexer.WindowSpec, _ bool) (*session.Session, error) {
	resp, err := c.sessions.CreateWithWindows(ctx, connect.NewRequest(&daemonv1.CreateWithWindowsRequest{
		Name:    req.Name,
		Remote:  req.Remote,
		ShCmd:   req.ShCmd,
		Windows: wire.WindowsToProto(windows),
	}))
	if err != nil {
		return nil, wire.FromConnectError(err)
	}
	sess := wire.SessionFromProto(resp.Msg.GetSession())
	return &sess, nil
}

func (c *Client) RenameSession(ctx context.Context, id, name string) error {
	_, err := c.sessions.Rename(ctx, connect.NewRequest(&daemonv1.RenameRequest{Id: id, Name: name}))
	return wire.FromConnectError(err)
}

func (c *Client) SetSessionGroup(ctx context.Context, id, group string) error {
	_, err := c.sessions.SetGroup(ctx, connect.NewRequest(&daemonv1.SetGroupRequest{Id: id, Group: group}))
	return wire.FromConnectError(err)
}

func (c *Client) DeleteSession(ctx context.Context, id string) error {
	_, err := c.sessions.Delete(ctx, connect.NewRequest(&daemonv1.DeleteRequest{Id: id}))
	return wire.FromConnectError(err)
}

func (c *Client) RecycleSession(ctx context.Context, id string, w io.Writer) error {
	stream, err := c.sessions.Recycle(ctx, connect.NewRequest(&daemonv1.RecycleRequest{Id: id}))
	if err != nil {
		return wire.FromConnectError(err)
	}
	defer func() { _ = stream.Close() }()
	for stream.Receive() {
		if w != nil {
			_, _ = fmt.Fprintln(w, stream.Msg().GetProgressLine())
		}
	}
	return wire.FromConnectError(stream.Err())
}

func (c *Client) Prune(ctx context.Context, all bool) (int, error) {
	resp, err := c.sessions.Prune(ctx, connect.NewRequest(&daemonv1.PruneRequest{All: all}))
	if err != nil {
		return 0, wire.FromConnectError(err)
	}
	return int(resp.Msg.GetCount()), nil
}

func (c *Client) CheckSessionRisk(ctx context.Context, id string) (sessionsvc.Risk, error) {
	resp, err := retryRead(func() (*connect.Response[daemonv1.RiskResponse], error) {
		return c.sessions.Risk(ctx, connect.NewRequest(&daemonv1.RiskRequest{Id: id}))
	})
	if err != nil {
		return sessionsvc.Risk{}, wire.FromConnectError(err)
	}
	return sessionsvc.Risk{
		UncommittedChanges: resp.Msg.GetUncommittedChanges(),
		UnpushedCommits:    resp.Msg.GetUnpushedCommits(),
	}, nil
}

func (c *Client) SessionLaunchOptions(ctx context.Context) (sessionsvc.LaunchOptions, error) {
	resp, err := retryRead(func() (*connect.Response[daemonv1.LaunchOptionsResponse], error) {
		return c.sessions.LaunchOptions(ctx, connect.NewRequest(&daemonv1.LaunchOptionsRequest{}))
	})
	if err != nil {
		return sessionsvc.LaunchOptions{}, wire.FromConnectError(err)
	}
	return wire.LaunchOptionsFromProto(resp.Msg), nil
}

// StatusAvailable answers from the last Hello. The first call starts a Hello
// in the background and answers false, so the TUI's Update never waits on
// the socket.
func (c *Client) StatusAvailable() bool {
	c.helloOnce.Do(func() {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), statusTimeout)
			defer cancel()
			if _, err := c.Hello(ctx); err != nil {
				c.logger.Debug().Err(err).Msg("hello for status availability failed")
			}
		}()
	})
	return c.statusAvailable.Load()
}

// FetchBatch logs a failure and returns no statuses, the same as an
// unavailable status service in-process.
func (c *Client) FetchBatch(ctx context.Context, sessions []*session.Session, roots []status.RootRepoTarget) map[string]status.TerminalStatus {
	if len(sessions) == 0 && len(roots) == 0 {
		return map[string]status.TerminalStatus{}
	}
	ids := make([]string, 0, len(sessions))
	for _, s := range sessions {
		ids = append(ids, s.ID)
	}
	ctx, cancel := context.WithTimeout(ctx, statusTimeout)
	defer cancel()
	req := &daemonv1.StatusesRequest{SessionIds: ids, Roots: wire.RootsToProto(roots)}
	resp, err := retryRead(func() (*connect.Response[daemonv1.StatusesResponse], error) {
		return c.sessions.Statuses(ctx, connect.NewRequest(req))
	})
	if err != nil {
		c.logger.Debug().Err(err).Msg("fetch statuses from the daemon")
		return map[string]status.TerminalStatus{}
	}
	return wire.StatusesFromProto(resp.Msg.GetStatuses())
}
