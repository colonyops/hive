package daemon_test

import (
	"io"
	"net/http"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	daemonv1 "github.com/colonyops/hive/internal/hive/daemon/gen/hive/daemon/v1"
	"github.com/colonyops/hive/internal/hive/daemon/gen/hive/daemon/v1/daemonv1connect"
)

func TestMetricsCountsRPCs(t *testing.T) {
	r := runDaemon(t, t.TempDir(), false)
	client := httpClient(r.socket)

	sessions := daemonv1connect.NewSessionServiceClient(client, "http://hived")
	_, err := sessions.List(t.Context(), connect.NewRequest(&daemonv1.ListRequest{}))
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://hived/metrics", nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	text := string(body)
	assert.Contains(t, text, `hive_daemon_rpc_requests_total{code="ok",procedure="/hive.daemon.v1.SessionService/List"} 1`)
	assert.Contains(t, text, `hive_daemon_rpc_duration_seconds_count{procedure="/hive.daemon.v1.SessionService/List"} 1`)
	assert.Contains(t, text, `hive_daemon_sessions{state="active"} 0`)
	assert.Contains(t, text, "hive_daemon_start_time_seconds ")
}
