package mcpsrv_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/adapter/mcpsrv"
	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/agentws"
	"github.com/colonyops/hive/cmd/desktop/internal/app/apptest"
)

const chatsToken = "test-token"

func testChatsSession(t *testing.T, seedConfig ...func(t *testing.T, configDir string)) (*app.App, *mcp.ClientSession) {
	t.Helper()
	cfg := apptest.Config(t)
	for _, seed := range seedConfig {
		require.NoError(t, os.MkdirAll(cfg.Paths.ConfigDir, 0o755))
		seed(t, cfg.Paths.ConfigDir)
	}
	core, err := app.New(t.Context(), cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = core.Close() })

	srv := mcpsrv.NewChats(core, zerolog.Nop(), mcpsrv.Options{Version: "test"}, chatsToken).Server()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	_, err = srv.Connect(t.Context(), serverTransport, nil)
	require.NoError(t, err)
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil).
		Connect(t.Context(), clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	return core, session
}

type chatRow struct {
	ID        string `json:"id"`
	Workspace string `json:"workspace"`
	Name      string `json:"name"`
	Running   bool   `json:"running"`
	PinnedAt  int64  `json:"pinnedAt"`
}

func TestChatPinsKeepPinOrderAndReadBack(t *testing.T) {
	t.Parallel()
	core, session := testChatsSession(t, seedWorkspace("demo", "version: 5\nname: Demo\ncommand: claude"+agentws.PromptTail+"\n", nil))
	first := seedAgentSession(t, core, "demo", "First")
	second := seedAgentSession(t, core, "demo", "Second")

	var a, b chatRow
	call(t, session, "chat_pin_put", map[string]any{"id": first}, &a)
	call(t, session, "chat_pin_put", map[string]any{"id": second}, &b)
	require.Positive(t, a.PinnedAt)
	assert.GreaterOrEqual(t, b.PinnedAt, a.PinnedAt)

	var again chatRow
	call(t, session, "chat_pin_put", map[string]any{"id": first}, &again)
	assert.Equal(t, a.PinnedAt, again.PinnedAt, "a pinned chat keeps its place")

	var list struct{ Chats []chatRow }
	call(t, session, "chat_list", map[string]any{"workspace": "demo"}, &list)
	pinned := map[string]int64{}
	for _, c := range list.Chats {
		pinned[c.ID] = c.PinnedAt
	}
	assert.Equal(t, map[string]int64{first: a.PinnedAt, second: b.PinnedAt}, pinned)

	var unpinned chatRow
	call(t, session, "chat_pin_delete", map[string]any{"id": first}, &unpinned)
	assert.Zero(t, unpinned.PinnedAt)
	call(t, session, "chat_pin_delete", map[string]any{"id": first}, &unpinned)
	assert.Zero(t, unpinned.PinnedAt)

	assert.True(t, strings.HasPrefix(callErr(t, session, "chat_pin_put", map[string]any{"id": "nope"}), "not_found:"))
	assert.True(t, strings.HasPrefix(callErr(t, session, "chat_list", map[string]any{"workspace": "nope"}), "not_found:"))
}

// These refusals all happen before a terminal starts, so the test needs no tmux.
func TestChatCreateRefusesBeforeItStartsAnything(t *testing.T) {
	t.Parallel()
	_, session := testChatsSession(t,
		seedWorkspace("prompted", "version: 5\nname: Prompted\ncommand: claude"+agentws.PromptTail+"\n", nil),
		seedWorkspace("plain", "version: 5\nname: Plain\ncommand: claude\n", nil),
	)

	cases := map[string]struct {
		args map[string]any
		kind string
	}{
		"missing workspace":    {map[string]any{"workspace": "nope", "name": "x"}, "not_found:"},
		"blank name":           {map[string]any{"workspace": "prompted", "name": "  "}, "invalid:"},
		"prompt with no slot":  {map[string]any{"workspace": "plain", "name": "x", "prompt": "do it"}, "invalid:"},
		"unknown field":        {map[string]any{"workspace": "prompted", "name": "x", "caller": "c"}, "invalid:"},
		"missing name in args": {map[string]any{"workspace": "prompted"}, "invalid:"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			text := callErr(t, session, "chat_create", tc.args)
			assert.True(t, strings.HasPrefix(text, tc.kind), text)
		})
	}

	var list struct{ Chats []chatRow }
	call(t, session, "chat_list", map[string]any{}, &list)
	assert.Empty(t, list.Chats, "a refused start leaves no chat record")
}

func TestChatsHandlerRequiresTheToken(t *testing.T) {
	t.Parallel()
	core, err := app.New(t.Context(), apptest.Config(t))
	require.NoError(t, err)
	t.Cleanup(func() { _ = core.Close() })
	srv := httptest.NewServer(mcpsrv.NewChats(core, zerolog.Nop(), mcpsrv.Options{Version: "test"}, chatsToken).Handler())
	t.Cleanup(srv.Close)

	for name, header := range map[string]string{"none": "", "wrong": "Bearer other", "no scheme": chatsToken} {
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

	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		r = r.Clone(r.Context())
		r.Header.Set("Authorization", "Bearer "+chatsToken)
		return http.DefaultTransport.RoundTrip(r)
	})}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil).Connect(t.Context(),
		&mcp.StreamableClientTransport{Endpoint: srv.URL, HTTPClient: client, DisableStandaloneSSE: true}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	tools, err := session.ListTools(t.Context(), nil)
	require.NoError(t, err)
	assert.Len(t, tools.Tools, 4)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestNewChatsRefusesAnEmptyToken(t *testing.T) {
	t.Parallel()
	assert.Panics(t, func() { mcpsrv.NewChats(nil, zerolog.Nop(), mcpsrv.Options{}, "") })
}
