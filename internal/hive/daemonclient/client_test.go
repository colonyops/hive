package daemonclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	daemonv1 "github.com/colonyops/hive/internal/hive/daemon/gen/hive/daemon/v1"
	"github.com/colonyops/hive/internal/hive/daemon/gen/hive/daemon/v1/daemonv1connect"
	"github.com/colonyops/hive/internal/hive/daemon/wire"
)

// countingServer counts the calls that reach it.
type countingServer struct {
	daemonv1connect.UnimplementedSessionServiceHandler
	listCalls   atomic.Int32
	deleteCalls atomic.Int32
}

func (s *countingServer) List(context.Context, *connect.Request[daemonv1.ListRequest]) (*connect.Response[daemonv1.ListResponse], error) {
	s.listCalls.Add(1)
	return connect.NewResponse(&daemonv1.ListResponse{}), nil
}

func (s *countingServer) Delete(context.Context, *connect.Request[daemonv1.DeleteRequest]) (*connect.Response[daemonv1.DeleteResponse], error) {
	s.deleteCalls.Add(1)
	return connect.NewResponse(&daemonv1.DeleteResponse{}), nil
}

// droppingListener closes its first drops connections once the client has
// written to them, the way a daemon killed during a call looks to the client.
type droppingListener struct {
	net.Listener
	drops   atomic.Int32
	dropped atomic.Int32
}

func (l *droppingListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		if l.drops.Add(-1) < 0 {
			return conn, nil
		}
		l.dropped.Add(1)
		buf := make([]byte, 1)
		_, _ = conn.Read(buf)
		_ = conn.Close()
	}
}

func socketPath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "hivec")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, "d.sock")
}

func serve(t *testing.T, path string, handler daemonv1connect.SessionServiceHandler, drops int32) *droppingListener {
	t.Helper()
	return serveWithControl(t, path, handler, &helloServer{}, drops)
}

func serveWithControl(t *testing.T, path string, handler daemonv1connect.SessionServiceHandler, control daemonv1connect.DaemonServiceHandler, drops int32) *droppingListener {
	t.Helper()
	inner, err := net.Listen("unix", path)
	require.NoError(t, err)
	listener := &droppingListener{Listener: inner}
	listener.drops.Store(drops)
	mux := http.NewServeMux()
	mux.Handle(daemonv1connect.NewSessionServiceHandler(handler))
	mux.Handle(daemonv1connect.NewDaemonServiceHandler(control))
	var protocols http.Protocols
	protocols.SetUnencryptedHTTP2(true)
	server := &http.Server{Handler: mux, Protocols: &protocols, ReadHeaderTimeout: time.Second}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	return listener
}

func TestClientRetriesAReadOnce(t *testing.T) {
	path := socketPath(t)
	server := &countingServer{}
	listener := serve(t, path, server, 1)
	client := New(Options{Dialer: UnixDialer(path), Logger: zerolog.Nop()})

	_, err := client.ListSessions(t.Context())
	require.NoError(t, err)
	assert.Equal(t, int32(1), listener.dropped.Load())
	assert.Equal(t, int32(1), server.listCalls.Load())
}

func TestClientGivesUpOnAReadAfterOneRetry(t *testing.T) {
	path := socketPath(t)
	listener := serve(t, path, &countingServer{}, 5)
	client := New(Options{Dialer: UnixDialer(path), Logger: zerolog.Nop()})

	_, err := client.ListSessions(t.Context())
	require.Error(t, err)
	assert.Equal(t, int32(2), listener.dropped.Load())
}

func TestClientDoesNotRetryAMutation(t *testing.T) {
	path := socketPath(t)
	server := &countingServer{}
	listener := serve(t, path, server, 1)
	client := New(Options{Dialer: UnixDialer(path), Logger: zerolog.Nop()})

	require.Error(t, client.DeleteSession(t.Context(), "s1"))
	assert.Equal(t, int32(1), listener.dropped.Load(), "a delete that may have applied must not run twice")
	assert.Zero(t, server.deleteCalls.Load())
}

func TestClientStartsTheDaemonWhenTheSocketIsMissing(t *testing.T) {
	path := socketPath(t)
	var starts atomic.Int32
	client := New(Options{
		Dialer: UnixDialer(path),
		Logger: zerolog.Nop(),
		AutoStart: func(context.Context) error {
			starts.Add(1)
			serve(t, path, &countingServer{}, 0)
			return nil
		},
	})

	_, err := client.ListSessions(t.Context())
	require.NoError(t, err)
	_, err = client.ListSessions(t.Context())
	require.NoError(t, err)
	assert.Equal(t, int32(1), starts.Load())
}

func TestClientWithoutAutoStartReportsTheDialError(t *testing.T) {
	client := New(Options{Dialer: UnixDialer(socketPath(t)), Logger: zerolog.Nop()})
	_, err := client.ListSessions(t.Context())
	require.Error(t, err)
	assert.Equal(t, connect.CodeUnavailable, connect.CodeOf(err))
}

func TestStarterBacksOffAfterAFailedStart(t *testing.T) {
	now := time.Unix(1000, 0)
	var starts int
	spawnErr := errors.New("no hive binary")
	s := &starter{
		start: func(context.Context) error { starts++; return spawnErr },
		now:   func() time.Time { return now },
	}
	down := func() bool { return false }

	var waits []time.Duration
	for range 8 {
		require.ErrorIs(t, s.ensure(t.Context(), down), spawnErr)
		require.ErrorIs(t, s.ensure(t.Context(), down), spawnErr, "inside the backoff the last error comes back")
		waits = append(waits, s.retryAt.Sub(now))
		now = s.retryAt
	}
	assert.Equal(t, []time.Duration{
		time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second,
		16 * time.Second, 30 * time.Second, 30 * time.Second, 30 * time.Second,
	}, waits)
	assert.Equal(t, 8, starts, "one start per expired backoff")

	s.start = func(context.Context) error { return nil }
	require.NoError(t, s.ensure(t.Context(), down))
	assert.Zero(t, s.backoff, "a success resets the backoff")
}

func TestStarterSkipsTheStartWhenADaemonAlreadyAnswers(t *testing.T) {
	s := &starter{start: func(context.Context) error { t.Fatal("started a second daemon"); return nil }, now: time.Now}
	require.NoError(t, s.ensure(t.Context(), func() bool { return true }))
}

type helloServer struct {
	daemonv1connect.UnimplementedDaemonServiceHandler
	calls atomic.Int32
}

func (s *helloServer) Hello(context.Context, *connect.Request[daemonv1.HelloRequest]) (*connect.Response[daemonv1.HelloResponse], error) {
	s.calls.Add(1)
	return connect.NewResponse(&daemonv1.HelloResponse{StatusAvailable: true}), nil
}

func TestStatusAvailableRetriesAHelloThatFailed(t *testing.T) {
	path := socketPath(t)
	now := time.Unix(1000, 0)
	var nowMu sync.Mutex
	client := New(Options{Dialer: UnixDialer(path), Logger: zerolog.Nop()})
	client.now = func() time.Time { nowMu.Lock(); defer nowMu.Unlock(); return now }
	idle := func() bool {
		client.helloMu.Lock()
		defer client.helloMu.Unlock()
		return !client.helloInFlight
	}

	assert.False(t, client.StatusAvailable(), "no daemon answers yet")
	require.Eventually(t, idle, 5*time.Second, 10*time.Millisecond)

	inner, err := net.Listen("unix", path)
	require.NoError(t, err)
	server := &helloServer{}
	mux := http.NewServeMux()
	mux.Handle(daemonv1connect.NewDaemonServiceHandler(server))
	var protocols http.Protocols
	protocols.SetUnencryptedHTTP2(true)
	srv := &http.Server{Handler: mux, Protocols: &protocols, ReadHeaderTimeout: time.Second}
	go func() { _ = srv.Serve(inner) }()
	t.Cleanup(func() { _ = srv.Close() })

	assert.False(t, client.StatusAvailable())
	assert.Zero(t, server.calls.Load(), "a retry waits out helloRetry")

	nowMu.Lock()
	now = now.Add(helloRetry)
	nowMu.Unlock()
	require.Eventually(t, client.StatusAvailable, 5*time.Second, 10*time.Millisecond)
	assert.Equal(t, int32(1), server.calls.Load())
}

// slowServer blocks List until the caller gives up, and answers Delete and
// Recycle only after slow.
type slowServer struct {
	daemonv1connect.UnimplementedSessionServiceHandler
	slow time.Duration
}

func (slowServer) List(ctx context.Context, _ *connect.Request[daemonv1.ListRequest]) (*connect.Response[daemonv1.ListResponse], error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (s slowServer) Delete(ctx context.Context, _ *connect.Request[daemonv1.DeleteRequest]) (*connect.Response[daemonv1.DeleteResponse], error) {
	select {
	case <-time.After(s.slow):
		return connect.NewResponse(&daemonv1.DeleteResponse{}), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s slowServer) Recycle(ctx context.Context, _ *connect.Request[daemonv1.RecycleRequest], stream *connect.ServerStream[daemonv1.RecycleResponse]) error {
	select {
	case <-time.After(s.slow):
		return stream.Send(&daemonv1.RecycleResponse{ProgressLine: "done"})
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestClientReadTimeoutBoundsOnlyReads(t *testing.T) {
	const timeout = 50 * time.Millisecond
	path := socketPath(t)
	serve(t, path, slowServer{slow: 4 * timeout}, 0)
	client := New(Options{Dialer: UnixDialer(path), Logger: zerolog.Nop(), ReadTimeout: timeout})

	_, err := client.ListSessions(t.Context())
	assert.Equal(t, connect.CodeDeadlineExceeded, connect.CodeOf(err))

	require.NoError(t, client.DeleteSession(t.Context(), "s1"), "a delete outlives the read timeout")
	require.NoError(t, client.RecycleSession(t.Context(), "s1", nil), "a stream outlives the read timeout")
}

func TestClientDeleteTimeoutBoundsADelete(t *testing.T) {
	const timeout = 50 * time.Millisecond
	path := socketPath(t)
	serve(t, path, slowServer{slow: time.Minute}, 0)
	client := New(Options{Dialer: UnixDialer(path), Logger: zerolog.Nop(), DeleteTimeout: timeout})

	err := client.DeleteSession(t.Context(), "s1")

	assert.ErrorContains(t, err, "the change may still finish on the server")
}

type stuckHelloServer struct {
	daemonv1connect.UnimplementedDaemonServiceHandler
}

func (stuckHelloServer) Hello(ctx context.Context, _ *connect.Request[daemonv1.HelloRequest]) (*connect.Response[daemonv1.HelloResponse], error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestClientStopsAChangeWhenTheDaemonIsStuck(t *testing.T) {
	path := socketPath(t)
	sessions := &countingServer{}
	serveWithControl(t, path, sessions, stuckHelloServer{}, 0)
	client := New(Options{Dialer: UnixDialer(path), Logger: zerolog.Nop(), ReadTimeout: 50 * time.Millisecond})

	err := client.DeleteSession(t.Context(), "s1")

	require.ErrorContains(t, err, "nothing changed")
	assert.Zero(t, sessions.deleteCalls.Load())
}

func TestExplain(t *testing.T) {
	timedOut := connect.NewError(connect.CodeDeadlineExceeded, context.DeadlineExceeded)
	refused := connect.NewError(connect.CodeUnavailable, errors.New("dial: EOF"))
	dropped := connect.NewError(connect.CodeUnknown, fmt.Errorf("protocol error: %w", io.ErrUnexpectedEOF))
	const read, change = daemonv1connect.SessionServiceGetProcedure, daemonv1connect.SessionServiceRenameProcedure

	tests := []struct {
		name      string
		err       error
		procedure string
		detail    string
		want      string
	}{
		{"timeout", timedOut, read, "", "box did not answer in 10s"},
		{"timeout with ssh detail", timedOut, read, "Connection timed out", "box did not answer in 10s: Connection timed out"},
		{"timeout during a change", timedOut, change, "", "box did not answer in 10s; the change may still finish on the server"},
		{"no connection", refused, read, "", "could not reach box: dial: EOF"},
		{"no connection with ssh detail", refused, read, "Permission denied", "could not reach box: Permission denied"},
		{"dropped connection", dropped, read, "", "lost the connection to box: protocol error: unexpected EOF"},
		{"dropped connection during a change", dropped, change, "", "lost the connection to box: protocol error: unexpected EOF; the change may still finish on the server"},
		{"a failed ssh looks like EOF", connect.NewError(connect.CodeUnavailable, io.ErrUnexpectedEOF), read, "Could not resolve hostname", "could not reach box: Could not resolve hostname"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New(Options{Logger: zerolog.Nop(), Name: "box", FailureDetail: func() string { return tt.detail }})

			err := c.explain(tt.err, tt.procedure, 10*time.Second)

			require.EqualError(t, err, tt.want)
			var unreachable *wire.UnreachableError
			require.ErrorAs(t, err, &unreachable)
			assert.Equal(t, "box", unreachable.Server)
			assert.Equal(t, connect.CodeOf(tt.err), connect.CodeOf(err))
			assert.ErrorIs(t, wire.FromConnectError(err), err)
		})
	}

	t.Run("an error from the daemon stays as it is", func(t *testing.T) {
		c := New(Options{Logger: zerolog.Nop()})
		wireErr := connect.NewWireError(connect.CodeUnavailable, errors.New("tmux is gone"))

		assert.Same(t, wireErr, c.explain(wireErr, read, time.Second))
	})

	t.Run("a timeout the daemon sent back is still a timeout", func(t *testing.T) {
		c := New(Options{Logger: zerolog.Nop(), Name: "box"})
		wireErr := connect.NewWireError(connect.CodeDeadlineExceeded, context.DeadlineExceeded)

		assert.EqualError(t, c.explain(wireErr, read, 10*time.Second), "box did not answer in 10s")
	})

	t.Run("an explained error stays as it is", func(t *testing.T) {
		c := New(Options{Logger: zerolog.Nop(), Name: "box"})
		explained := c.explain(timedOut, read, 10*time.Second)

		assert.Same(t, explained, c.explain(explained, change, 20*time.Second))
	})

	t.Run("a local daemon has no server name", func(t *testing.T) {
		c := New(Options{Logger: zerolog.Nop()})

		assert.EqualError(t, c.explain(timedOut, read, 10*time.Second), "the daemon did not answer in 10s")
	})
}

func TestTimeoutFor(t *testing.T) {
	c := New(Options{
		Logger:         zerolog.Nop(),
		ReadTimeout:    time.Second,
		ConnectTimeout: 2 * time.Second,
		RiskTimeout:    3 * time.Second,
		DeleteTimeout:  4 * time.Second,
	})
	get := daemonv1connect.SessionServiceGetProcedure

	assert.Equal(t, 2*time.Second, c.timeoutFor(get), "no connection yet")
	assert.Equal(t, 3*time.Second, c.timeoutFor(daemonv1connect.SessionServiceRiskProcedure))
	assert.Equal(t, 4*time.Second, c.timeoutFor(daemonv1connect.SessionServiceDeleteProcedure))
	assert.Zero(t, c.timeoutFor(daemonv1connect.SessionServiceRenameProcedure))

	c.openConns.Add(1)
	assert.Equal(t, time.Second, c.timeoutFor(get), "has a connection")
}

func TestClientWithoutReadTimeoutSendsNoHelloBeforeAChange(t *testing.T) {
	path := socketPath(t)
	sessions := &countingServer{}
	control := &helloServer{}
	serveWithControl(t, path, sessions, control, 0)
	client := New(Options{Dialer: UnixDialer(path), Logger: zerolog.Nop()})

	require.NoError(t, client.DeleteSession(t.Context(), "s1"))

	assert.Zero(t, control.calls.Load())
	assert.Equal(t, int32(1), sessions.deleteCalls.Load())
}
