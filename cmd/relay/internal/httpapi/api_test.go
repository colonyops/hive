package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/relay/internal/relay"
	"github.com/colonyops/hive/internal/web"
)

type client struct {
	h   *harness
	key string
}

func (c client) call(method, path string, body any) *httptest.ResponseRecorder {
	c.h.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(c.h.t, json.NewEncoder(&buf).Encode(body))
	}
	r := newRequest(method, path, &buf)
	if c.key != "" {
		r.Header.Set("Authorization", "Bearer "+c.key)
	}
	return c.h.do(r)
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &v), rec.Body.String())
	return v
}

func TestAPIRequiresBearer(t *testing.T) {
	h := newHarness(t, relay.DefaultLimits())

	rec := client{h: h}.call(http.MethodGet, "/v1/sources", nil)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, "unauthorized", decode[web.ErrorBody](t, rec).Kind)

	rec = client{h: h, key: "bogus"}.call(http.MethodGet, "/v1/sources", nil)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, testVersion, rec.Header().Get(VersionHeader))
}

func TestAPIUnauthenticatedRoutes(t *testing.T) {
	h := newHarness(t, relay.DefaultLimits())
	anon := client{h: h}

	rec := anon.call(http.MethodGet, "/v1/auth/methods", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"email_code":false,"password":false,"api_key":true}`, rec.Body.String())

	rec = anon.call(http.MethodGet, "/healthz", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "ok", rec.Body.String())
}

func TestAPISourcesAndDeliveries(t *testing.T) {
	h := newHarness(t, relay.DefaultLimits())
	c := client{h: h, key: h.mint(owner)}

	rec := c.call(http.MethodPost, "/v1/sources", map[string]string{"name": "ci", "secret": "abc"})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	created := decode[createdSourceResponse](t, rec)
	assert.Equal(t, "ci", created.Name)
	assert.True(t, created.HasSecret)
	require.True(t, strings.HasPrefix(created.IngressURL, testPublicURL+"/in/"))
	assert.NotContains(t, rec.Body.String(), "hash")
	token := strings.TrimPrefix(created.IngressURL, testPublicURL+"/in/")

	rec = c.call(http.MethodPost, "/v1/sources", map[string]string{"name": ""})
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	rec = c.call(http.MethodGet, "/v1/sources", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	list := decode[sourcesResponse](t, rec)
	require.Len(t, list.Sources, 1)
	assert.Equal(t, created.ID, list.Sources[0].ID)

	rec = c.call(http.MethodPatch, "/v1/sources/"+created.ID, map[string]string{"name": "ci-2", "secret": ""})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	patched := decode[sourceView](t, rec)
	assert.Equal(t, "ci-2", patched.Name)
	assert.False(t, patched.HasSecret)

	for i := range 3 {
		require.Equal(t, http.StatusOK, h.do(ingressPost(token, strings.Repeat("x", i+1))).Code)
	}

	rec = c.call(http.MethodGet, "/v1/sources/"+created.ID, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	got := decode[sourceView](t, rec)
	assert.Equal(t, 3, got.Deliveries)
	assert.Positive(t, got.Bytes)

	deliveries := "/v1/sources/" + created.ID + "/deliveries"
	rec = c.call(http.MethodGet, deliveries+"?limit=2", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	page := decode[relay.Page](t, rec)
	require.Len(t, page.Deliveries, 2)
	assert.True(t, page.More)
	assert.Equal(t, "x", page.Deliveries[0].Body)

	rec = c.call(http.MethodGet, deliveries+"?limit=2&after="+page.Next, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	page = decode[relay.Page](t, rec)
	require.Len(t, page.Deliveries, 1)
	assert.False(t, page.More)
	assert.Equal(t, "xxx", page.Deliveries[0].Body)

	assert.Equal(t, http.StatusUnprocessableEntity, c.call(http.MethodGet, deliveries+"?limit=501", nil).Code)
	assert.Equal(t, http.StatusUnprocessableEntity, c.call(http.MethodGet, deliveries+"?after=nope", nil).Code)
	assert.Equal(t, http.StatusBadRequest, c.call(http.MethodGet, deliveries+"?limit=many", nil).Code)

	rec = c.call(http.MethodPost, "/v1/sources/"+created.ID+"/rotate", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	rotated := decode[ingressURLResponse](t, rec)
	assert.NotEqual(t, created.IngressURL, rotated.IngressURL)
	assert.Equal(t, http.StatusNotFound, h.do(ingressPost(token, "x")).Code)
	newToken := strings.TrimPrefix(rotated.IngressURL, testPublicURL+"/in/")
	assert.Equal(t, http.StatusOK, h.do(ingressPost(newToken, "x")).Code)

	other := client{h: h, key: h.mint("other@example.com")}
	assert.Equal(t, http.StatusNotFound, other.call(http.MethodGet, "/v1/sources/"+created.ID, nil).Code)
	assert.Equal(t, http.StatusNotFound, other.call(http.MethodGet, deliveries, nil).Code)
	assert.Equal(t, http.StatusNotFound, other.call(http.MethodDelete, "/v1/sources/"+created.ID, nil).Code)

	assert.Equal(t, http.StatusNoContent, c.call(http.MethodDelete, "/v1/sources/"+created.ID, nil).Code)
	rec = c.call(http.MethodGet, "/v1/sources/"+created.ID, nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, "not_found", decode[web.ErrorBody](t, rec).Kind)
}

func TestAPITokens(t *testing.T) {
	h := newHarness(t, relay.DefaultLimits())
	c := client{h: h, key: h.mint(owner)}

	rec := c.call(http.MethodPost, "/v1/tokens", map[string]string{"name": "laptop"})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	created := decode[createdTokenResponse](t, rec)
	assert.NotEmpty(t, created.Token)
	assert.Equal(t, "laptop", created.Name)
	assert.Equal(t, relay.TokenAPI, created.Kind)

	assert.Equal(t, http.StatusUnprocessableEntity, c.call(http.MethodPost, "/v1/tokens", map[string]string{"name": " "}).Code)

	rec = c.call(http.MethodGet, "/v1/tokens", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, rec.Body.String(), "hash")
	list := decode[tokensResponse](t, rec)
	require.Len(t, list.Tokens, 2)
	var current int
	for _, tok := range list.Tokens {
		if tok.Current {
			current++
			assert.NotEqual(t, created.ID, tok.ID)
		}
	}
	assert.Equal(t, 1, current)

	minted := client{h: h, key: created.Token}
	assert.Equal(t, http.StatusOK, minted.call(http.MethodGet, "/v1/me", nil).Code)

	assert.Equal(t, http.StatusNoContent, c.call(http.MethodDelete, "/v1/tokens/"+created.ID, nil).Code)
	assert.Equal(t, http.StatusUnauthorized, minted.call(http.MethodGet, "/v1/me", nil).Code)
	assert.Equal(t, http.StatusNotFound, c.call(http.MethodDelete, "/v1/tokens/"+created.ID, nil).Code)
}

func TestAPIMe(t *testing.T) {
	h := newHarness(t, relay.DefaultLimits())
	c := client{h: h, key: h.mint(owner)}
	_, token := h.source(owner, "")
	require.Equal(t, http.StatusOK, h.do(ingressPost(token, "hello")).Code)

	rec := c.call(http.MethodGet, "/v1/me", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	me := decode[meResponse](t, rec)
	assert.Equal(t, owner, me.Email)
	assert.False(t, me.Encryption)
	assert.Equal(t, 1, me.Usage.Sources)
	assert.Positive(t, me.Usage.Bytes)
	assert.Equal(t, int64(14*24*60*60), me.Limits.RetentionSeconds)
	assert.Equal(t, int64(256<<10), me.Limits.BodyBytes)
	assert.True(t, me.Methods.APIKey)
	assert.Equal(t, testVersion, me.Version)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
	for _, key := range []string{"email", "encryption", "limits", "usage", "methods", "version"} {
		assert.Contains(t, raw, key)
	}
}
