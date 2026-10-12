package httpapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/relay/internal/config"
	"github.com/colonyops/hive/cmd/relay/internal/relay"
	"github.com/colonyops/hive/cmd/relay/internal/store/memory"
)

const (
	testVersion   = "test-1"
	testPublicURL = "http://relay.test"
)

type harness struct {
	t    *testing.T
	api  *api
	core Core
}

func newHarness(t *testing.T, limits relay.Limits, trusted ...netip.Prefix) *harness {
	t.Helper()
	store := memory.New()
	t.Cleanup(func() { _ = store.Close() })
	core := Core{
		Auth:    relay.NewAuth(store, nil),
		Sources: relay.NewSources(store, limits, testPublicURL),
		Ingress: relay.NewIngress(store, limits),
		Feed:    relay.NewFeed(store),
		Store:   store,
		Limits:  limits,
		Version: testVersion,
	}
	cfg := config.Config{TrustedProxies: trusted, Limits: limits}
	return &harness{t: t, api: newAPI(core, cfg, zerolog.Nop()), core: core}
}

func (h *harness) do(r *http.Request) *httptest.ResponseRecorder {
	h.t.Helper()
	if r.RemoteAddr == "" {
		r.RemoteAddr = "192.0.2.1:5000"
	}
	rec := httptest.NewRecorder()
	h.api.handler.ServeHTTP(rec, r)
	return rec
}

// source creates a source for email directly through the core and returns it
// with its ingress token.
func (h *harness) source(email, secret string) (relay.Source, string) {
	h.t.Helper()
	src, token, err := h.core.Sources.Create(context.Background(), email, "test", secret)
	require.NoError(h.t, err)
	return src, token
}

func (h *harness) mint(email string) string {
	h.t.Helper()
	key, _, err := h.core.Auth.MintToken(context.Background(), email, "test", relay.TokenAPI)
	require.NoError(h.t, err)
	return key
}

func newRequest(method, target string, body io.Reader) *http.Request {
	return httptest.NewRequestWithContext(context.Background(), method, target, body)
}

func ingressPost(token, body string) *http.Request {
	return newRequest(http.MethodPost, "/in/"+token, strings.NewReader(body))
}
