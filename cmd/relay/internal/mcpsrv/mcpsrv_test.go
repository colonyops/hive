package mcpsrv_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"filippo.io/age"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/relay/internal/config"
	"github.com/colonyops/hive/cmd/relay/internal/httpapi"
	"github.com/colonyops/hive/cmd/relay/internal/mcpsrv"
	"github.com/colonyops/hive/cmd/relay/internal/relay"
	"github.com/colonyops/hive/cmd/relay/internal/store/memory"
	"github.com/colonyops/hive/cmd/relay/internal/wire"
	"github.com/colonyops/hive/internal/web"
)

const (
	testEmail   = "agent@example.com"
	testVersion = "test-1"
)

type env struct {
	t     *testing.T
	store relay.Store
	auth  *relay.Auth
	srv   *httptest.Server
}

func newEnv(t *testing.T) *env {
	t.Helper()
	store := memory.New()
	t.Cleanup(func() { _ = store.Close() })
	limits := relay.DefaultLimits()
	srv := httptest.NewUnstartedServer(nil)
	publicURL := "http://" + srv.Listener.Addr().String()
	allow := relay.NewAllowlist(nil, nil)
	core := httpapi.Core{
		Auth:     relay.NewAuth(store, nil),
		SignIn:   relay.NewSignIn(store, allow, nil, store.Outbox(), publicURL, false),
		Sources:  relay.NewSources(store, limits, publicURL),
		Ingress:  relay.NewIngress(store, limits),
		Feed:     relay.NewFeed(store),
		Accounts: relay.NewAccounts(store),
		Store:    store,
		Limits:   limits,
		Version:  testVersion,
	}
	core.MCP = mcpsrv.New(mcpsrv.Core{
		Auth: core.Auth, SignIn: core.SignIn, Sources: core.Sources, Feed: core.Feed, Accounts: core.Accounts,
		Store: store, Limits: limits, Version: testVersion,
	}, zerolog.Nop()).Handler()
	srv.Config.Handler = httpapi.New(core, config.Config{Limits: limits}, zerolog.Nop())
	srv.Start()
	t.Cleanup(srv.Close)
	return &env{t: t, store: store, auth: core.Auth, srv: srv}
}

func (e *env) mint() string {
	e.t.Helper()
	key, _, err := e.auth.MintToken(context.Background(), testEmail, "test", relay.TokenAPI)
	require.NoError(e.t, err)
	return key
}

type bearerTransport struct{ key string }

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.key)
	return http.DefaultTransport.RoundTrip(r)
}

func (e *env) connect(key string) (*mcp.ClientSession, error) {
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	return client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:             e.srv.URL + "/mcp",
		HTTPClient:           &http.Client{Transport: bearerTransport{key: key}, Timeout: 10 * time.Second},
		MaxRetries:           -1,
		DisableStandaloneSSE: true,
	}, nil)
}

func (e *env) session(key string) *mcp.ClientSession {
	e.t.Helper()
	cs, err := e.connect(key)
	require.NoError(e.t, err)
	e.t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func callRaw(t *testing.T, cs *mcp.ClientSession, name string, args any) *mcp.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	require.NoError(t, err)
	return res
}

func text(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

func call[T any](t *testing.T, cs *mcp.ClientSession, name string, args any) T {
	t.Helper()
	res := callRaw(t, cs, name, args)
	require.False(t, res.IsError, text(res))
	var out T
	require.NoError(t, json.Unmarshal([]byte(text(res)), &out), text(res))
	return out
}

func callErr(t *testing.T, cs *mcp.ClientSession, name string, args any) string {
	t.Helper()
	res := callRaw(t, cs, name, args)
	require.True(t, res.IsError, text(res))
	return text(res)
}

func TestToolsList(t *testing.T) {
	e := newEnv(t)
	cs := e.session(e.mint())
	res, err := cs.ListTools(context.Background(), nil)
	require.NoError(t, err)
	names := make([]string, 0, len(res.Tools))
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	assert.ElementsMatch(t, []string{
		"whoami", "list_sources", "create_source", "update_source", "delete_source",
		"rotate_ingress", "list_deliveries", "list_tokens", "create_token", "revoke_token",
		"set_encryption", "list_recipients", "add_recipient", "remove_recipient",
	}, names)
}

func TestWhoami(t *testing.T) {
	e := newEnv(t)
	cs := e.session(e.mint())
	me := call[wire.Me](t, cs, "whoami", nil)
	assert.Equal(t, testEmail, me.Email)
	assert.Equal(t, testVersion, me.Version)
	assert.True(t, me.Methods.APIKey)
	assert.Equal(t, relay.DefaultLimits().SourcesPerAccount, me.Limits.SourcesPerAccount)
}

func TestSourceTools(t *testing.T) {
	e := newEnv(t)
	cs := e.session(e.mint())

	created := call[wire.CreatedSource](t, cs, "create_source", map[string]any{"name": "ci", "secret": "s3cret"})
	assert.Equal(t, "ci", created.Name)
	assert.True(t, created.HasSecret)
	assert.True(t, strings.HasPrefix(created.IngressURL, e.srv.URL+"/in/"), created.IngressURL)

	list := call[wire.Sources](t, cs, "list_sources", nil)
	require.Len(t, list.Sources, 1)
	assert.Equal(t, created.ID, list.Sources[0].ID)

	updated := call[wire.Source](t, cs, "update_source", map[string]any{"id": created.ID, "name": "builds", "secret": ""})
	assert.Equal(t, "builds", updated.Name)
	assert.False(t, updated.HasSecret)

	rotated := call[wire.IngressURL](t, cs, "rotate_ingress", map[string]any{"id": created.ID})
	assert.NotEqual(t, created.IngressURL, rotated.IngressURL)

	call[map[string]any](t, cs, "delete_source", map[string]any{"id": created.ID})
	assert.Empty(t, call[wire.Sources](t, cs, "list_sources", nil).Sources)

	assert.True(t, strings.HasPrefix(callErr(t, cs, "delete_source", map[string]any{"id": created.ID}), "not_found"))
}

func TestInputValidation(t *testing.T) {
	e := newEnv(t)
	cs := e.session(e.mint())

	assert.True(t, strings.HasPrefix(callErr(t, cs, "create_source", map[string]any{"name": " "}), "invalid"))
	assert.True(t, strings.HasPrefix(callErr(t, cs, "create_source", map[string]any{"name": strings.Repeat("x", 81)}), "invalid"))
	assert.True(t, strings.HasPrefix(callErr(t, cs, "create_source", map[string]any{"name": "ok", "secret": "tab\there"}), "invalid"))

	src := call[wire.CreatedSource](t, cs, "create_source", map[string]any{"name": "ok"})
	assert.True(t, strings.HasPrefix(callErr(t, cs, "list_deliveries", map[string]any{"source_id": src.ID, "limit": 501}), "invalid"))
	assert.True(t, strings.HasPrefix(callErr(t, cs, "list_deliveries", map[string]any{"source_id": src.ID, "after": "nope"}), "invalid"))
	assert.True(t, strings.HasPrefix(callErr(t, cs, "create_token", map[string]any{"name": ""}), "invalid"))
}

func (e *env) deliver(ingressURL, body string) {
	e.t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, ingressURL, strings.NewReader(body))
	require.NoError(e.t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(e.t, err)
	_ = resp.Body.Close()
	require.Less(e.t, resp.StatusCode, 300)
}

func TestListDeliveries(t *testing.T) {
	e := newEnv(t)
	cs := e.session(e.mint())
	src := call[wire.CreatedSource](t, cs, "create_source", map[string]any{"name": "hooks"})
	e.deliver(src.IngressURL, `{"n":1}`)
	e.deliver(src.IngressURL, `{"n":2}`)

	first := call[relay.Page](t, cs, "list_deliveries", map[string]any{"source_id": src.ID, "limit": 1})
	require.Len(t, first.Deliveries, 1)
	assert.True(t, first.More)
	assert.JSONEq(t, `{"n":1}`, first.Deliveries[0].Body)

	rest := call[relay.Page](t, cs, "list_deliveries", map[string]any{"source_id": src.ID, "after": first.Next})
	require.Len(t, rest.Deliveries, 1)
	assert.False(t, rest.More)
	assert.JSONEq(t, `{"n":2}`, rest.Deliveries[0].Body)
}

func TestListDeliveriesIsMetadataOnlyWhenEncrypted(t *testing.T) {
	e := newEnv(t)
	cs := e.session(e.mint())
	src := call[wire.CreatedSource](t, cs, "create_source", map[string]any{"name": "hooks"})
	e.deliver(src.IngressURL, `{"secret":"payload"}`)

	ctx := context.Background()
	acct, err := e.store.Accounts().Get(ctx, testEmail)
	require.NoError(t, err)
	acct.Encryption = true
	require.NoError(t, e.store.Accounts().Put(ctx, acct))

	res := callRaw(t, cs, "list_deliveries", map[string]any{"source_id": src.ID})
	require.False(t, res.IsError, text(res))
	assert.NotContains(t, text(res), "payload")
	var page struct {
		Deliveries []map[string]any `json:"deliveries"`
	}
	require.NoError(t, json.Unmarshal([]byte(text(res)), &page))
	require.Len(t, page.Deliveries, 1)
	keys := make([]string, 0, len(page.Deliveries[0]))
	for k := range page.Deliveries[0] {
		keys = append(keys, k)
	}
	assert.ElementsMatch(t, []string{"id", "received_at", "content_length"}, keys)
}

func TestTokenTools(t *testing.T) {
	e := newEnv(t)
	cs := e.session(e.mint())

	created := call[wire.CreatedToken](t, cs, "create_token", map[string]any{"name": "workspace:demo"})
	assert.NotEmpty(t, created.Token)
	assert.Equal(t, relay.TokenAPI, created.Kind)

	toks := call[wire.Tokens](t, cs, "list_tokens", nil)
	require.Len(t, toks.Tokens, 2)
	current := 0
	for _, tok := range toks.Tokens {
		if tok.Current {
			current++
			assert.NotEqual(t, created.ID, tok.ID)
		}
	}
	assert.Equal(t, 1, current)
	assert.NotContains(t, strings.ToLower(text(callRaw(t, cs, "list_tokens", nil))), "hash")

	minted := e.session(created.Token)
	assert.Equal(t, testEmail, call[wire.Me](t, minted, "whoami", nil).Email)

	call[map[string]any](t, cs, "revoke_token", map[string]any{"id": created.ID})
	_, err := e.connect(created.Token)
	require.Error(t, err)
}

func TestMCPRequiresBearer(t *testing.T) {
	e := newEnv(t)
	for _, key := range []string{"", "bogus"} {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, e.srv.URL+"/mcp",
			strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		var body web.ErrorBody
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		_ = resp.Body.Close()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.Equal(t, "unauthorized", body.Kind)
		assert.Equal(t, testVersion, resp.Header.Get(httpapi.VersionHeader))
	}
}

func TestEncryptionAndRecipientTools(t *testing.T) {
	e := newEnv(t)
	cs := e.session(e.mint())
	identity, err := age.GenerateX25519Identity()
	require.NoError(t, err)

	assert.Contains(t, callErr(t, cs, "set_encryption", map[string]any{"on": true}), "invalid")
	assert.Contains(t, callErr(t, cs, "add_recipient", map[string]any{"recipient": "nope"}), "invalid")

	added := call[wire.Recipient](t, cs, "add_recipient", map[string]any{"recipient": identity.Recipient().String(), "device": "laptop"})
	assert.Equal(t, "laptop", added.Device)
	list := call[wire.Recipients](t, cs, "list_recipients", nil)
	assert.Equal(t, []wire.Recipient{added}, list.Recipients)

	me := call[wire.Me](t, cs, "set_encryption", map[string]any{"on": true})
	assert.True(t, me.Encryption)
	assert.True(t, call[wire.Me](t, cs, "whoami", nil).Encryption)

	assert.Contains(t, callErr(t, cs, "remove_recipient", map[string]any{"id": added.ID}), "invalid")
	assert.False(t, call[wire.Me](t, cs, "set_encryption", map[string]any{"on": false}).Encryption)
	gone := call[map[string]any](t, cs, "remove_recipient", map[string]any{"id": added.ID})
	assert.Equal(t, true, gone["deleted"])
	assert.Contains(t, callErr(t, cs, "remove_recipient", map[string]any{"id": added.ID}), "not_found")
	assert.Empty(t, call[wire.Recipients](t, cs, "list_recipients", nil).Recipients)
}
