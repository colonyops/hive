package daemonclient

import (
	"context"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/hive/daemon/gen/hive/daemon/v1/daemonv1connect"
	"github.com/colonyops/hive/internal/hive/daemon/wire"
)

type echoIn struct {
	Word string `json:"word"`
}

type echoOut struct {
	Word string `json:"word"`
}

// serveEngine serves Hello, the session service and an engine MCP server
// with one tool, echo, and counts the tool's calls.
func serveEngine(t *testing.T, catalog int32) (string, *countingServer, *atomic.Int32) {
	t.Helper()
	path := socketPath(t)
	sessions := &countingServer{}
	var calls atomic.Int32
	srv := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "test"}, nil)
	mcp.AddTool(srv, &mcp.Tool{Name: "echo"}, func(_ context.Context, _ *mcp.CallToolRequest, in echoIn) (*mcp.CallToolResult, echoOut, error) {
		calls.Add(1)
		return nil, echoOut(in), nil
	})

	listener, err := net.Listen("unix", path)
	require.NoError(t, err)
	mux := http.NewServeMux()
	mux.Handle(daemonv1connect.NewDaemonServiceHandler(versionedServer{protocol: wire.ProtocolVersion, catalog: catalog}))
	mux.Handle(daemonv1connect.NewSessionServiceHandler(sessions))
	mux.Handle(wire.EnginePath, mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true}))
	var protocols http.Protocols
	protocols.SetUnencryptedHTTP2(true)
	server := &http.Server{Handler: mux, Protocols: &protocols, ReadHeaderTimeout: time.Second}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	return path, sessions, &calls
}

func engineClient(path string) *Client {
	return New(Options{Dialer: UnixDialer(path), Logger: zerolog.Nop(), CheckProtocol: true, ClientVersion: "v0.9.0", ReadTimeout: 5 * time.Second})
}

func TestCallEngineRunsTheToolOnTheDaemon(t *testing.T) {
	t.Parallel()
	path, _, calls := serveEngine(t, wire.CatalogVersion)

	res, err := engineClient(path).CallEngine(t.Context(), EngineCall{Tool: "echo", Args: map[string]any{"word": "hi"}, Read: true})

	require.NoError(t, err)
	require.False(t, res.IsError)
	assert.Equal(t, map[string]any{"word": "hi"}, res.StructuredContent)
	assert.Equal(t, int32(1), calls.Load())
}

func TestCallEngineRefusesADaemonOutsideTheCatalogRange(t *testing.T) {
	t.Parallel()
	for _, catalog := range []int32{0, wire.CatalogVersion + 1} {
		path, sessions, calls := serveEngine(t, catalog)
		client := engineClient(path)

		_, err := client.CallEngine(t.Context(), EngineCall{Tool: "echo", Args: map[string]any{"word": "hi"}})

		var refused *wire.CatalogError
		require.ErrorAs(t, err, &refused, "catalog %d", catalog)
		assert.Contains(t, err.Error(), "v0.1.0")
		assert.Contains(t, err.Error(), "v0.9.0")
		assert.Zero(t, calls.Load(), "the call never reaches the daemon's engine")

		_, err = client.ListSessions(t.Context())
		require.NoError(t, err, "session calls do not depend on the catalog")
		assert.Equal(t, int32(1), sessions.listCalls.Load())
	}
}

func TestCallEngineNamesBothVersionsForAToolTheDaemonLacks(t *testing.T) {
	t.Parallel()
	path, _, _ := serveEngine(t, wire.CatalogVersion)

	_, err := engineClient(path).CallEngine(t.Context(), EngineCall{Tool: "added_later"})

	var unknown *wire.UnknownToolError
	require.ErrorAs(t, err, &unknown)
	assert.Contains(t, err.Error(), "v0.1.0")
	assert.Contains(t, err.Error(), "v0.9.0")
}

func TestCallEngineNamesTheServerWhenItCannotReachIt(t *testing.T) {
	t.Parallel()
	client := New(Options{Dialer: UnixDialer(socketPath(t)), Logger: zerolog.Nop(), CheckProtocol: true, Name: "box"})

	_, err := client.CallEngine(t.Context(), EngineCall{Tool: "echo", Read: true})

	var unreachable *wire.UnreachableError
	require.ErrorAs(t, err, &unreachable)
	assert.Contains(t, err.Error(), "box")
}
