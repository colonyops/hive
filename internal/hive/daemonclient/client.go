// Package daemonclient implements sessionapi.API against a hive daemon.
package daemonclient

import (
	"context"
	"errors"
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
	"github.com/colonyops/hive/internal/hive/daemon/daemonpath"
	daemonv1 "github.com/colonyops/hive/internal/hive/daemon/gen/hive/daemon/v1"
	"github.com/colonyops/hive/internal/hive/daemon/gen/hive/daemon/v1/daemonv1connect"
	"github.com/colonyops/hive/internal/hive/daemon/wire"
	"github.com/colonyops/hive/internal/hive/gitstatus"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/internal/hive/sessionapi"
	"github.com/colonyops/hive/internal/hive/status"
	"github.com/colonyops/hive/pkg/logutils"
)

// Dialer opens one connection to a daemon.
type Dialer func(ctx context.Context) (net.Conn, error)

// LocalDialer reaches the daemon that serves dataDir on this machine.
func LocalDialer(dataDir string) Dialer {
	return UnixDialer(daemonpath.SocketPath(dataDir))
}

func UnixDialer(path string) Dialer {
	return func(ctx context.Context) (net.Conn, error) {
		if err := daemonpath.CheckSocketDir(path); err != nil {
			return nil, err
		}
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
	// ReadTimeout bounds each read-only call, so a slow remote daemon cannot
	// block a caller. A call that changes sessions first asks the daemon for
	// Hello under this bound, so a stuck daemon fails it before any change.
	// Zero sets no bound and sends no Hello.
	ReadTimeout time.Duration
	// ConnectTimeout replaces ReadTimeout for a read while the client has no
	// connection, since setting one up can take many round trips on a slow link.
	ConnectTimeout time.Duration
	// RiskTimeout replaces ReadTimeout for a risk check, which runs git and takes
	// as long as the repository needs.
	RiskTimeout time.Duration
	// DeleteTimeout bounds a delete. The service finishes a delete it has
	// started, so a cut-off delete cannot leave a half-deleted session. Other
	// calls that change sessions have no bound: a clone can take any time, and
	// a rename cut off midway leaves tmux and the store apart.
	DeleteTimeout time.Duration
	// Name is the server this client reaches, for its errors. Empty means the
	// daemon on this machine.
	Name string
	// FailureDetail adds the transport's reason to an error, such as the last
	// line ssh printed.
	FailureDetail func() string
	// CheckProtocol refuses every call to a daemon whose protocol this
	// client does not support, with an error that names ClientVersion and
	// the daemon's version. A remote daemon can be any version; a local one
	// is the binary that spawned it.
	CheckProtocol bool
	ClientVersion string
}

// baseURL is never resolved: every connection goes through the Dialer.
const baseURL = "http://hived"

// statusTimeout bounds FetchBatch, which has no error to return.
const statusTimeout = 5 * time.Second

type Client struct {
	logger     zerolog.Logger
	dialer     Dialer
	starter    *starter
	protocol   *protocolGate
	httpClient *http.Client
	sessions   daemonv1connect.SessionServiceClient
	control    daemonv1connect.DaemonServiceClient

	name          string
	failureDetail func() string

	checkBeforeChange bool
	readTimeout       time.Duration
	connectTimeout    time.Duration
	riskTimeout       time.Duration
	deleteTimeout     time.Duration
	openConns         atomic.Int32

	statusAvailable atomic.Bool
	helloMu         sync.Mutex
	helloInFlight   bool
	lastHello       time.Time
	now             func() time.Time
}

var _ sessionapi.API = (*Client)(nil)

func New(opts Options) *Client {
	c := &Client{
		logger: logutils.Component(opts.Logger, "daemon-client"),
		dialer: opts.Dialer,
		now:    time.Now,

		name:          opts.Name,
		failureDetail: opts.FailureDetail,

		checkBeforeChange: opts.ReadTimeout > 0,
		readTimeout:       opts.ReadTimeout,
		connectTimeout:    opts.ConnectTimeout,
		riskTimeout:       opts.RiskTimeout,
		deleteTimeout:     opts.DeleteTimeout,
	}
	if opts.AutoStart != nil {
		c.starter = &starter{start: opts.AutoStart, now: time.Now}
	}
	var protocols http.Protocols
	protocols.SetUnencryptedHTTP2(true)
	c.httpClient = &http.Client{Transport: &http.Transport{
		Protocols: &protocols,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			conn, err := c.dial(ctx)
			if err != nil {
				return nil, err
			}
			c.openConns.Add(1)
			return &countedConn{Conn: conn, closed: func() { c.openConns.Add(-1) }}, nil
		},
	}}
	interceptors := []connect.Interceptor{c.explainFailures()}
	if opts.CheckProtocol {
		c.protocol = &protocolGate{client: c, clientVersion: opts.ClientVersion}
		interceptors = append(interceptors, c.protocol)
	}
	// Inside the gate: its Hello has its own limit, so connecting on a slow link
	// does not use up the limit of the call behind it.
	interceptors = append(interceptors, c.callTimeouts())
	clientOpts := []connect.ClientOption{connect.WithInterceptors(interceptors...)}
	c.sessions = daemonv1connect.NewSessionServiceClient(c.httpClient, baseURL, clientOpts...)
	c.control = daemonv1connect.NewDaemonServiceClient(c.httpClient, baseURL, clientOpts...)
	return c
}

var readProcedures = map[string]bool{
	daemonv1connect.DaemonServiceHelloProcedure:          true,
	daemonv1connect.DaemonServiceHealthProcedure:         true,
	daemonv1connect.SessionServiceListProcedure:          true,
	daemonv1connect.SessionServiceGetProcedure:           true,
	daemonv1connect.SessionServiceRiskProcedure:          true,
	daemonv1connect.SessionServiceLaunchOptionsProcedure: true,
	daemonv1connect.SessionServiceStatusesProcedure:      true,
	daemonv1connect.SessionServiceChangesProcedure:       true,
	daemonv1connect.SessionServiceFileDiffsProcedure:     true,
	daemonv1connect.SessionServiceBranchesProcedure:      true,
	daemonv1connect.SessionServiceGitStatusesProcedure:   true,
}

// fileDiffsTimeout outlasts the daemon's own 15s limit on a batch of diffs, so
// the daemon's answer arrives before the client gives up.
const fileDiffsTimeout = 20 * time.Second

func (c *Client) callTimeouts() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			timeout := c.timeoutFor(req.Spec().Procedure)
			if timeout == 0 {
				return next(ctx, req)
			}
			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			return next(ctx, req)
		}
	}
}

func (c *Client) timeoutFor(procedure string) time.Duration {
	switch {
	case procedure == daemonv1connect.SessionServiceDeleteProcedure:
		return c.deleteTimeout
	case !readProcedures[procedure]:
		return 0
	case procedure == daemonv1connect.SessionServiceRiskProcedure && c.riskTimeout > 0:
		return c.riskTimeout
	case c.connectTimeout > 0 && c.openConns.Load() == 0:
		return c.connectTimeout
	case procedure == daemonv1connect.SessionServiceFileDiffsProcedure && c.readTimeout > 0:
		return max(c.readTimeout, fileDiffsTimeout)
	}
	return c.readTimeout
}

// checkAnswering fails a change before it starts when the daemon is stuck. An
// ssh connection to a stuck daemon stays healthy, so nothing else notices.
func (c *Client) checkAnswering(ctx context.Context) error {
	if !c.checkBeforeChange {
		return nil
	}
	if _, err := c.Hello(ctx); err != nil {
		return fmt.Errorf("%w; nothing changed", err)
	}
	return nil
}

// HTTPClient reaches the daemon's plain HTTP endpoints, such as /healthz.
func (c *Client) HTTPClient() *http.Client { return c.httpClient }

type Info struct {
	Version         string
	ProtocolVersion int
	PID             int
	DataDir         string
	StartedAt       time.Time
	StatusAvailable bool
}

// AgentCheck is where the daemon finds an agent profile's command. Path is
// empty when the command is not on the daemon's PATH.
type AgentCheck struct {
	Profile string
	Command string
	Path    string
}

type StatusLoopHealth struct {
	LastTick     time.Time
	LastDuration time.Duration
	Errors       int64
	Paused       bool
}

type Health struct {
	Supervised      bool
	Agents          []AgentCheck
	DBOK            bool
	DBError         string
	TmuxOK          bool
	StatusLoop      StatusLoopHealth
	SessionsByState map[string]int
	Uptime          time.Duration
}

// Hello also refreshes the cached answer StatusAvailable gives.
func (c *Client) Hello(ctx context.Context) (Info, error) {
	info, err := c.helloOnce(ctx)
	if err != nil && transportFailure(err) {
		return c.helloOnce(ctx)
	}
	return info, err
}

func (c *Client) helloOnce(ctx context.Context) (Info, error) {
	resp, err := c.control.Hello(ctx, connect.NewRequest(&daemonv1.HelloRequest{}))
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
		ProtocolVersion: int(msg.GetProtocolVersion()),
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
		Supervised:      msg.GetSupervised(),
	}
	for _, agent := range msg.GetAgents() {
		health.Agents = append(health.Agents, AgentCheck{Profile: agent.GetProfile(), Command: agent.GetCommand(), Path: agent.GetPath()})
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

// Metrics returns the daemon's /metrics page in the Prometheus text format.
func (c *Client) Metrics(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/metrics", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("daemon /metrics: %s", resp.Status)
	}
	return body, nil
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

func (c *Client) CreateSession(ctx context.Context, opts sessionsvc.CreateOptions) (*session.Session, error) {
	if err := c.checkAnswering(ctx); err != nil {
		return nil, err
	}
	began := c.now()
	stream, err := c.sessions.Create(ctx, connect.NewRequest(wire.CreateOptionsToProto(opts)))
	if err != nil {
		return nil, c.streamFailure(err, daemonv1connect.SessionServiceCreateProcedure, began)
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
		return nil, c.streamFailure(err, daemonv1connect.SessionServiceCreateProcedure, began)
	}
	if created == nil {
		return nil, fmt.Errorf("create session: the daemon ended the stream without a session")
	}
	return created, nil
}

func (c *Client) CreateFromRequest(ctx context.Context, req sessionsvc.LaunchRequest) (session.Session, error) {
	if err := c.checkAnswering(ctx); err != nil {
		return session.Session{}, err
	}
	resp, err := c.sessions.CreateFromRequest(ctx, connect.NewRequest(wire.LaunchRequestToProto(req)))
	if err != nil {
		return session.Session{}, wire.FromConnectError(err)
	}
	return wire.SessionFromProto(resp.Msg.GetSession()), nil
}

func (c *Client) CreateSessionWithWindows(ctx context.Context, req sessionsvc.NewSessionRequest, windows []multiplexer.WindowSpec) (*session.Session, error) {
	if err := c.checkAnswering(ctx); err != nil {
		return nil, err
	}
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
	if err := c.checkAnswering(ctx); err != nil {
		return err
	}
	_, err := c.sessions.Rename(ctx, connect.NewRequest(&daemonv1.RenameRequest{Id: id, Name: name}))
	return wire.FromConnectError(err)
}

func (c *Client) SetSessionGroup(ctx context.Context, id, group string) error {
	if err := c.checkAnswering(ctx); err != nil {
		return err
	}
	_, err := c.sessions.SetGroup(ctx, connect.NewRequest(&daemonv1.SetGroupRequest{Id: id, Group: group}))
	return wire.FromConnectError(err)
}

func (c *Client) SetSessionTags(ctx context.Context, id string, tags []string) (session.Session, error) {
	if err := c.checkAnswering(ctx); err != nil {
		return session.Session{}, err
	}
	resp, err := c.sessions.SetTags(ctx, connect.NewRequest(&daemonv1.SetTagsRequest{Id: id, Tags: tags}))
	if err != nil {
		return session.Session{}, wire.FromConnectError(err)
	}
	return wire.SessionFromProto(resp.Msg.GetSession()), nil
}

func (c *Client) EditSessionTags(ctx context.Context, id string, add, remove []string) (session.Session, error) {
	if err := c.checkAnswering(ctx); err != nil {
		return session.Session{}, err
	}
	resp, err := c.sessions.EditTags(ctx, connect.NewRequest(&daemonv1.EditTagsRequest{Id: id, Add: add, Remove: remove}))
	if err != nil {
		return session.Session{}, wire.FromConnectError(err)
	}
	return wire.SessionFromProto(resp.Msg.GetSession()), nil
}

func (c *Client) DeleteSession(ctx context.Context, id string) error {
	if err := c.checkAnswering(ctx); err != nil {
		return err
	}
	_, err := c.sessions.Delete(ctx, connect.NewRequest(&daemonv1.DeleteRequest{Id: id}))
	return wire.FromConnectError(err)
}

func (c *Client) RecycleSession(ctx context.Context, id string, w io.Writer) error {
	if err := c.checkAnswering(ctx); err != nil {
		return err
	}
	began := c.now()
	stream, err := c.sessions.Recycle(ctx, connect.NewRequest(&daemonv1.RecycleRequest{Id: id}))
	if err != nil {
		return c.streamFailure(err, daemonv1connect.SessionServiceRecycleProcedure, began)
	}
	defer func() { _ = stream.Close() }()
	for stream.Receive() {
		if w != nil {
			_, _ = fmt.Fprintln(w, stream.Msg().GetProgressLine())
		}
	}
	return c.streamFailure(stream.Err(), daemonv1connect.SessionServiceRecycleProcedure, began)
}

func (c *Client) Prune(ctx context.Context, all bool) (int, error) {
	if err := c.checkAnswering(ctx); err != nil {
		return 0, err
	}
	resp, err := c.sessions.Prune(ctx, connect.NewRequest(&daemonv1.PruneRequest{All: all}))
	if err != nil {
		return 0, wire.FromConnectError(err)
	}
	return int(resp.Msg.GetCount()), nil
}

func (c *Client) CheckSessionRisk(ctx context.Context, id string) (sessionsvc.Risk, error) {
	if err := c.checkAnswering(ctx); err != nil {
		return sessionsvc.Risk{}, err
	}
	resp, err := retryRead(func() (*connect.Response[daemonv1.RiskResponse], error) {
		return c.sessions.Risk(ctx, connect.NewRequest(&daemonv1.RiskRequest{Id: id}))
	})
	if connect.CodeOf(err) == connect.CodeDeadlineExceeded {
		return sessionsvc.Risk{}, fmt.Errorf("%w: %w", sessionapi.ErrRiskCheckTimedOut, err)
	}
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

func (c *Client) SessionChanges(ctx context.Context, id string, target gitstatus.DiffTarget) (gitstatus.Changes, error) {
	resp, err := retryRead(func() (*connect.Response[daemonv1.ChangesResponse], error) {
		return c.sessions.Changes(ctx, connect.NewRequest(&daemonv1.ChangesRequest{Id: id, Target: wire.DiffTargetToProto(target)}))
	})
	if err != nil {
		return gitstatus.Changes{}, wire.FromConnectError(err)
	}
	return wire.ChangesFromProto(resp.Msg), nil
}

func (c *Client) SessionFileDiffs(ctx context.Context, id string, target gitstatus.DiffTarget, paths []string) ([]gitstatus.FileDiff, error) {
	resp, err := retryRead(func() (*connect.Response[daemonv1.FileDiffsResponse], error) {
		return c.sessions.FileDiffs(ctx, connect.NewRequest(&daemonv1.FileDiffsRequest{Id: id, Target: wire.DiffTargetToProto(target), Paths: paths}))
	})
	if err != nil {
		return nil, wire.FromConnectError(err)
	}
	return wire.FileDiffsFromProto(resp.Msg), nil
}

func (c *Client) SessionBranches(ctx context.Context, id string) ([]string, error) {
	resp, err := retryRead(func() (*connect.Response[daemonv1.BranchesResponse], error) {
		return c.sessions.Branches(ctx, connect.NewRequest(&daemonv1.BranchesRequest{Id: id}))
	})
	if err != nil {
		return nil, wire.FromConnectError(err)
	}
	return resp.Msg.GetRefs(), nil
}

// ErrGitStatusUnsupported is a daemon whose hive predates GitStatuses.
var ErrGitStatusUnsupported = errors.New("the server's hive is too old to report git status; install the same hive version on both machines")

// GitStatuses reports a daemon that predates the RPC as such, so a user
// knows to upgrade the server rather than look for a fault.
func (c *Client) GitStatuses(ctx context.Context, ids []string) (map[string]gitstatus.Status, error) {
	if len(ids) == 0 {
		return map[string]gitstatus.Status{}, nil
	}
	resp, err := retryRead(func() (*connect.Response[daemonv1.GitStatusesResponse], error) {
		return c.sessions.GitStatuses(ctx, connect.NewRequest(&daemonv1.GitStatusesRequest{SessionIds: ids}))
	})
	if err != nil {
		if connect.CodeOf(err) == connect.CodeUnimplemented {
			return nil, ErrGitStatusUnsupported
		}
		return nil, wire.FromConnectError(err)
	}
	return wire.GitStatusesFromProto(resp.Msg.GetStatuses()), nil
}

// helloRetry spaces background Hellos while status is not yet known
// available, so a daemon that was down at the first call is seen once it is
// up.
const helloRetry = 2 * time.Second

// StatusAvailable answers from the last Hello. Until a Hello reports status
// available, a call starts one in the background (at most one at a time, and
// one per helloRetry) and answers false, so the TUI's Update never waits on
// the socket.
func (c *Client) StatusAvailable() bool {
	if c.statusAvailable.Load() {
		return true
	}
	c.helloMu.Lock()
	defer c.helloMu.Unlock()
	if c.helloInFlight || (!c.lastHello.IsZero() && c.now().Sub(c.lastHello) < helloRetry) {
		return false
	}
	c.helloInFlight = true
	c.lastHello = c.now()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), statusTimeout)
		defer cancel()
		if _, err := c.Hello(ctx); err != nil {
			c.logger.Debug().Err(err).Msg("hello for status availability failed")
		}
		c.helloMu.Lock()
		c.helloInFlight = false
		c.helloMu.Unlock()
	}()
	return false
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
