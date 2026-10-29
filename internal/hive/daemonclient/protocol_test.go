package daemonclient

import (
	"context"
	"errors"
	"net"
	"net/http"
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

type versionedServer struct {
	daemonv1connect.UnimplementedDaemonServiceHandler
	protocol int32
}

func (s versionedServer) Hello(context.Context, *connect.Request[daemonv1.HelloRequest]) (*connect.Response[daemonv1.HelloResponse], error) {
	return connect.NewResponse(&daemonv1.HelloResponse{Version: "v0.1.0", ProtocolVersion: s.protocol}), nil
}

// serveBoth serves the daemon service and the session service on path.
func serveBoth(t *testing.T, path string, control daemonv1connect.DaemonServiceHandler, sessions daemonv1connect.SessionServiceHandler) {
	t.Helper()
	listener, err := net.Listen("unix", path)
	require.NoError(t, err)
	mux := http.NewServeMux()
	mux.Handle(daemonv1connect.NewDaemonServiceHandler(control))
	mux.Handle(daemonv1connect.NewSessionServiceHandler(sessions))
	var protocols http.Protocols
	protocols.SetUnencryptedHTTP2(true)
	server := &http.Server{Handler: mux, Protocols: &protocols, ReadHeaderTimeout: time.Second}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
}

func TestCheckProtocolRefusesAnIncompatibleDaemon(t *testing.T) {
	path := socketPath(t)
	sessions := &countingServer{}
	serveBoth(t, path, versionedServer{protocol: 0}, sessions)
	client := New(Options{Dialer: UnixDialer(path), Logger: zerolog.Nop(), CheckProtocol: true, ClientVersion: "v0.9.0"})

	_, err := client.ListSessions(t.Context())

	var incompatible *wire.IncompatibleError
	require.ErrorAs(t, err, &incompatible)
	assert.Contains(t, err.Error(), "v0.1.0")
	assert.Contains(t, err.Error(), "v0.9.0")
	assert.Zero(t, sessions.listCalls.Load(), "the call never reaches the daemon")
}

func TestCheckProtocolPassesACompatibleDaemon(t *testing.T) {
	path := socketPath(t)
	sessions := &countingServer{}
	serveBoth(t, path, versionedServer{protocol: wire.ProtocolVersion}, sessions)
	client := New(Options{Dialer: UnixDialer(path), Logger: zerolog.Nop(), CheckProtocol: true})

	_, err := client.ListSessions(t.Context())
	require.NoError(t, err)
	_, err = client.ListSessions(t.Context())
	require.NoError(t, err)
	assert.Equal(t, int32(2), sessions.listCalls.Load())
}

func TestCheckProtocolDoesNotMultiplyRetries(t *testing.T) {
	var dials atomic.Int32
	client := New(Options{
		Logger:        zerolog.Nop(),
		CheckProtocol: true,
		Dialer: func(context.Context) (net.Conn, error) {
			dials.Add(1)
			return nil, errors.New("no route to host")
		},
	})

	_, err := client.ListSessions(t.Context())

	require.Error(t, err)
	assert.Equal(t, int32(2), dials.Load(), "the read retries once; its protocol check does not retry too")
}
