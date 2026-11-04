package daemonclient

import (
	"context"
	"net"
	"net/http"
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

// fakeHost runs fake daemons one at a time on one socket. A Stop ends the
// daemon the way the real one does: it answers, then closes its listener.
type fakeHost struct {
	t        *testing.T
	path     string
	sessions *countingServer
	stops    atomic.Int32
	spawns   atomic.Int32

	mu      sync.Mutex
	servers []*http.Server
}

type fakeDaemonService struct {
	daemonv1connect.UnimplementedDaemonServiceHandler
	host     *fakeHost
	server   *http.Server
	version  string
	protocol int32
}

func (s *fakeDaemonService) Hello(context.Context, *connect.Request[daemonv1.HelloRequest]) (*connect.Response[daemonv1.HelloResponse], error) {
	return connect.NewResponse(&daemonv1.HelloResponse{Version: s.version, ProtocolVersion: s.protocol}), nil
}

func (s *fakeDaemonService) Stop(context.Context, *connect.Request[daemonv1.StopRequest]) (*connect.Response[daemonv1.StopResponse], error) {
	s.host.stops.Add(1)
	go func() { _ = s.server.Shutdown(context.Background()) }()
	return connect.NewResponse(&daemonv1.StopResponse{}), nil
}

func newFakeHost(t *testing.T) *fakeHost {
	t.Helper()
	h := &fakeHost{t: t, path: socketPath(t), sessions: &countingServer{}}
	t.Cleanup(func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		for _, server := range h.servers {
			_ = server.Close()
		}
	})
	return h
}

func (h *fakeHost) start(version string, protocol int32) error {
	listener, err := net.Listen("unix", h.path)
	if err != nil {
		return err
	}
	service := &fakeDaemonService{host: h, version: version, protocol: protocol}
	mux := http.NewServeMux()
	mux.Handle(daemonv1connect.NewDaemonServiceHandler(service))
	mux.Handle(daemonv1connect.NewSessionServiceHandler(h.sessions))
	var protocols http.Protocols
	protocols.SetUnencryptedHTTP2(true)
	service.server = &http.Server{Handler: mux, Protocols: &protocols, ReadHeaderTimeout: time.Second}
	h.mu.Lock()
	h.servers = append(h.servers, service.server)
	h.mu.Unlock()
	go func() { _ = service.server.Serve(listener) }()
	return nil
}

// client returns a local client that spawns a fake daemon of spawnVersion.
func (h *fakeHost) client(clientVersion, spawnVersion string) *Client {
	return New(Options{
		Dialer:        UnixDialer(h.path),
		Logger:        zerolog.Nop(),
		CheckProtocol: true,
		ClientVersion: clientVersion,
		ReplaceOlder:  true,
		AutoStart: func(context.Context) error {
			h.spawns.Add(1)
			return h.start(spawnVersion, wire.ProtocolVersion)
		},
	})
}

func TestReplaceOlderRestartsAnOlderDaemon(t *testing.T) {
	t.Parallel()
	host := newFakeHost(t)
	require.NoError(t, host.start("v0.1.0", wire.ProtocolVersion))
	client := host.client("v0.2.0", "v0.2.0")

	_, err := client.ListSessions(t.Context())
	require.NoError(t, err)
	info, err := client.Hello(t.Context())
	require.NoError(t, err)

	assert.Equal(t, int32(1), host.stops.Load())
	assert.Equal(t, int32(1), host.spawns.Load())
	assert.Equal(t, "v0.2.0", info.Version)
	assert.Equal(t, int32(1), host.sessions.listCalls.Load(), "the call reaches the replacement")
}

func TestReplaceOlderComparesVersionsWithoutAPrefix(t *testing.T) {
	t.Parallel()
	host := newFakeHost(t)
	require.NoError(t, host.start("0.1.0", wire.ProtocolVersion))
	client := host.client("0.2.0", "0.2.0")

	_, err := client.ListSessions(t.Context())

	require.NoError(t, err)
	assert.Equal(t, int32(1), host.stops.Load())
}

func TestReplaceOlderRestartsOnlyOnce(t *testing.T) {
	t.Parallel()
	host := newFakeHost(t)
	require.NoError(t, host.start("v0.1.0", wire.ProtocolVersion))
	// The replacement is still older, as when a service unit names an old binary.
	client := host.client("v0.2.0", "v0.1.0")

	for range 3 {
		_, err := client.ListSessions(t.Context())
		require.NoError(t, err)
		// A new connection checks the daemon again.
		client.protocol.reset()
	}

	assert.Equal(t, int32(1), host.stops.Load())
	assert.Equal(t, int32(1), host.spawns.Load())
	assert.Equal(t, int32(3), host.sessions.listCalls.Load())
}

func TestReplaceOlderLeavesOtherDaemonsRunning(t *testing.T) {
	t.Parallel()
	cases := map[string]struct{ daemon, client string }{
		"newer daemon":         {"v0.3.0", "v0.2.0"},
		"equal":                {"v0.2.0", "v0.2.0"},
		"dev daemon":           {"dev", "v0.2.0"},
		"dev client":           {"v0.1.0", "dev"},
		"prerelease daemon":    {"0.1.0-5-gabc1234", "v0.2.0"},
		"unparseable daemon":   {"", "v0.2.0"},
		"unparseable client":   {"v0.1.0", "unknown"},
		"two-part daemon":      {"v0.1", "v0.2.0"},
		"both dev":             {"dev", "dev"},
		"prerelease client":    {"v0.1.0", "v0.2.0-rc1"},
		"build metadata equal": {"v0.2.0+abc", "v0.2.0"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			host := newFakeHost(t)
			require.NoError(t, host.start(tc.daemon, wire.ProtocolVersion))
			client := host.client(tc.client, tc.client)

			_, err := client.ListSessions(t.Context())

			require.NoError(t, err)
			assert.Zero(t, host.stops.Load())
			assert.Zero(t, host.spawns.Load())
			assert.Equal(t, int32(1), host.sessions.listCalls.Load())
		})
	}
}

func TestReplaceOlderStillRefusesAProtocolOutOfRange(t *testing.T) {
	t.Parallel()
	host := newFakeHost(t)
	require.NoError(t, host.start("v0.3.0", wire.ProtocolVersion+1))
	client := host.client("v0.2.0", "v0.2.0")

	_, err := client.ListSessions(t.Context())

	var incompatible *wire.IncompatibleError
	require.ErrorAs(t, err, &incompatible)
	assert.Contains(t, err.Error(), "v0.3.0")
	assert.Contains(t, err.Error(), "v0.2.0")
	assert.Zero(t, host.stops.Load())
	assert.Zero(t, host.sessions.listCalls.Load())
}

func TestReplaceOlderReplacesAnOlderDaemonWithAnUnsupportedProtocol(t *testing.T) {
	t.Parallel()
	host := newFakeHost(t)
	require.NoError(t, host.start("v0.1.0", wire.ProtocolVersion+1))
	client := host.client("v0.2.0", "v0.2.0")

	_, err := client.ListSessions(t.Context())

	require.NoError(t, err)
	assert.Equal(t, int32(1), host.stops.Load())
}

func TestReplaceOlderRestartsOnceForConcurrentCalls(t *testing.T) {
	t.Parallel()
	host := newFakeHost(t)
	require.NoError(t, host.start("v0.1.0", wire.ProtocolVersion))
	client := host.client("v0.2.0", "v0.2.0")

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Go(func() {
			_, err := client.ListSessions(t.Context())
			errs <- err
		})
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}
	assert.Equal(t, int32(1), host.stops.Load())
	assert.Equal(t, int32(1), host.spawns.Load())
}

func TestReplaceOlderIgnoresAClientWithoutAutoStart(t *testing.T) {
	t.Parallel()
	host := newFakeHost(t)
	require.NoError(t, host.start("v0.1.0", wire.ProtocolVersion))
	client := New(Options{
		Dialer:        UnixDialer(host.path),
		Logger:        zerolog.Nop(),
		CheckProtocol: true,
		ClientVersion: "v0.2.0",
		ReplaceOlder:  true,
	})

	_, err := client.ListSessions(t.Context())

	require.NoError(t, err)
	assert.Zero(t, host.stops.Load())
}

func TestOlderRelease(t *testing.T) {
	t.Parallel()
	assert.True(t, olderRelease("0.9.0", "0.60.0"), "compares numbers, not text")
	assert.True(t, olderRelease("v1.2.3", "1.2.4"))
	assert.False(t, olderRelease("1.2.4", "1.2.3"))
	assert.False(t, olderRelease("dev", "1.2.3"))
}
