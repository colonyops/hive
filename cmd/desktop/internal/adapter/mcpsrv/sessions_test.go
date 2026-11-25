package mcpsrv_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/adapter/mcpsrv"
	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/apptest"
	"github.com/colonyops/hive/cmd/desktop/internal/app/mcpcatalog"
)

func testSessionsSession(t *testing.T) (*app.App, *mcp.ClientSession) {
	t.Helper()
	core, err := app.New(t.Context(), apptest.Config(t))
	require.NoError(t, err)
	t.Cleanup(func() { _ = core.Close() })

	srv := mcpsrv.NewSessions(core, zerolog.Nop(), mcpsrv.Options{Version: "test"}, chatsToken).Server()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	_, err = srv.Connect(t.Context(), serverTransport, nil)
	require.NoError(t, err)
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil).
		Connect(t.Context(), clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	return core, session
}

func TestSessionsHandlerRequiresTheToken(t *testing.T) {
	t.Parallel()
	core, err := app.New(t.Context(), apptest.Config(t))
	require.NoError(t, err)
	t.Cleanup(func() { _ = core.Close() })
	srv := httptest.NewServer(mcpsrv.NewSessions(core, zerolog.Nop(), mcpsrv.Options{Version: "test"}, chatsToken).Handler())
	t.Cleanup(srv.Close)

	for name, header := range map[string]string{"none": "", "wrong": "Bearer other"} {
		t.Run(name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL, strings.NewReader(`{}`))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			if header != "" {
				req.Header.Set("Authorization", header)
			}
			res, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			_ = res.Body.Close()
			assert.Equal(t, http.StatusUnauthorized, res.StatusCode)
		})
	}
}

func TestNewSessionsRefusesAnEmptyToken(t *testing.T) {
	t.Parallel()
	assert.Panics(t, func() { mcpsrv.NewSessions(nil, zerolog.Nop(), mcpsrv.Options{}, "") })
}

func TestSessionsCatalogueAgreement(t *testing.T) {
	t.Parallel()
	entry, ok := mcpcatalog.Lookup("hive-sessions")
	require.True(t, ok)
	assert.Equal(t, mcpsrv.SessionsPathPrefix, entry.RuntimePath)
	assert.True(t, entry.StdioOnly)
}
