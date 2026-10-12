package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/relay/internal/relay"
	"github.com/colonyops/hive/internal/web"
)

var mailCodeRE = regexp.MustCompile(`\b\d{8}\b`)

func (h *harness) lastCode(email string) string {
	h.t.Helper()
	due, err := h.core.Store.Outbox().Due(context.Background(), time.Now().Add(time.Hour), 100)
	require.NoError(h.t, err)
	for i := len(due) - 1; i >= 0; i-- {
		if due[i].To == email {
			return mailCodeRE.FindString(due[i].Body)
		}
	}
	h.t.Fatalf("no mail for %s", email)
	return ""
}

func TestAuthEmailCodeFlow(t *testing.T) {
	h := newSignInHarness(t, relay.DefaultLimits(), signInSetup{patterns: []string{"*@example.com"}})
	anon := client{h: h}

	rec := anon.call(http.MethodGet, "/v1/auth/methods", nil)
	assert.JSONEq(t, `{"email_code":true,"password":false,"api_key":true}`, rec.Body.String())

	rec = anon.call(http.MethodPost, "/v1/auth/start", map[string]string{"email": "Alice@Example.com"})
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{}`, rec.Body.String())

	code := h.lastCode("alice@example.com")
	rec = anon.call(http.MethodPost, "/v1/auth/verify", map[string]string{"email": "alice@example.com", "code": code, "device": "laptop"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	got := decode[signedInResponse](t, rec)
	assert.Equal(t, "alice@example.com", got.Email)
	assert.Equal(t, relay.TokenDevice, got.Kind)
	assert.Equal(t, "laptop", got.Device)
	assert.NotEmpty(t, got.ID)

	me := client{h: h, key: got.Token}.call(http.MethodGet, "/v1/me", nil)
	require.Equal(t, http.StatusOK, me.Code)
	assert.True(t, decode[meResponse](t, me).Methods.EmailCode)

	rec = anon.call(http.MethodPost, "/v1/auth/verify", map[string]string{"email": "alice@example.com", "code": code})
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAuthStartRejects(t *testing.T) {
	h := newSignInHarness(t, relay.DefaultLimits(), signInSetup{patterns: []string{"*@example.com"}})
	anon := client{h: h}

	rec := anon.call(http.MethodPost, "/v1/auth/start", map[string]string{"email": "eve@evil.test"})
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Equal(t, "forbidden", decode[web.ErrorBody](t, rec).Kind)

	rec = anon.call(http.MethodPost, "/v1/auth/start", map[string]string{"email": "not-an-email"})
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())

	rec = anon.call(http.MethodPost, "/v1/auth/verify", map[string]string{"email": "a@example.com", "code": "1234"})
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())

	off := newHarness(t, relay.DefaultLimits())
	rec = client{h: off}.call(http.MethodPost, "/v1/auth/start", map[string]string{"email": "a@example.com"})
	assert.Equal(t, http.StatusForbidden, rec.Code, "email codes are off without a sender")
}

func TestAuthLogin(t *testing.T) {
	hash, err := relay.HashPassword("hunter2")
	require.NoError(t, err)
	h := newSignInHarness(t, relay.DefaultLimits(), signInSetup{users: map[string]string{"pw@example.com": hash}})
	anon := client{h: h}

	rec := anon.call(http.MethodGet, "/v1/auth/methods", nil)
	assert.JSONEq(t, `{"email_code":false,"password":true,"api_key":true}`, rec.Body.String())

	rec = anon.call(http.MethodPost, "/v1/auth/login", map[string]string{"email": "pw@example.com", "password": "hunter2", "device": "desk"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	got := decode[signedInResponse](t, rec)
	assert.Equal(t, relay.TokenDevice, got.Kind)
	assert.Equal(t, http.StatusOK, client{h: h, key: got.Token}.call(http.MethodGet, "/v1/me", nil).Code)

	rec = anon.call(http.MethodPost, "/v1/auth/login", map[string]string{"email": "pw@example.com", "password": "nope"})
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	rec = anon.call(http.MethodPost, "/v1/auth/login", map[string]string{"email": "other@example.com", "password": "hunter2"})
	assert.Equal(t, http.StatusForbidden, rec.Code)
	rec = anon.call(http.MethodPost, "/v1/auth/login", map[string]string{"email": "pw@example.com", "password": ""})
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}

func TestAuthKillSwitch(t *testing.T) {
	h := newSignInHarness(t, relay.DefaultLimits(), signInSetup{patterns: []string{"*@example.com"}})
	c := client{h: h, key: h.mint("alice@example.com")}
	require.Equal(t, http.StatusOK, c.call(http.MethodGet, "/v1/me", nil).Code)

	h.allow.Reload([]string{"bob@example.com"}, nil)
	rec := c.call(http.MethodGet, "/v1/me", nil)
	assert.Equal(t, http.StatusForbidden, rec.Code)

	h.allow.Reload([]string{"*@example.com"}, nil)
	assert.Equal(t, http.StatusOK, c.call(http.MethodGet, "/v1/me", nil).Code)
}

func tinyAuthLimits(perEmail, perIP int) relay.Limits {
	l := relay.DefaultLimits()
	l.AuthPerEmail = relay.Rate{Per: time.Hour, Count: perEmail, Burst: perEmail}
	l.AuthPerIP = relay.Rate{Per: time.Hour, Count: perIP, Burst: perIP}
	return l
}

func TestAuthRateLimitPerEmail(t *testing.T) {
	h := newSignInHarness(t, tinyAuthLimits(2, 100), signInSetup{patterns: []string{"*@example.com"}})
	start := func(email, ip string) *httptest.ResponseRecorder {
		r := newRequest(http.MethodPost, "/v1/auth/start", jsonBody(t, map[string]string{"email": email}))
		r.RemoteAddr = ip + ":5000"
		return h.do(r)
	}
	assert.Equal(t, http.StatusAccepted, start("a@example.com", "192.0.2.1").Code)
	assert.Equal(t, http.StatusAccepted, start("A@example.com", "192.0.2.2").Code)
	resp := start("a@example.com", "192.0.2.3")
	assert.Equal(t, http.StatusTooManyRequests, resp.Code)
	assert.NotEmpty(t, resp.Header().Get("Retry-After"))
	assert.Equal(t, http.StatusAccepted, start("b@example.com", "192.0.2.3").Code, "another email has its own bucket")
}

func TestAuthRateLimitPerIP(t *testing.T) {
	h := newSignInHarness(t, tinyAuthLimits(100, 2), signInSetup{patterns: []string{"*@example.com"}})
	verify := func(email, ip string) *httptest.ResponseRecorder {
		r := newRequest(http.MethodPost, "/v1/auth/verify", jsonBody(t, map[string]string{"email": email, "code": "12345678"}))
		r.RemoteAddr = ip + ":5000"
		return h.do(r)
	}
	assert.Equal(t, http.StatusUnauthorized, verify("a@example.com", "192.0.2.1").Code)
	assert.Equal(t, http.StatusUnauthorized, verify("b@example.com", "192.0.2.1").Code)
	resp := verify("c@example.com", "192.0.2.1")
	assert.Equal(t, http.StatusTooManyRequests, resp.Code)
	assert.NotEmpty(t, resp.Header().Get("Retry-After"))
	assert.Equal(t, http.StatusUnauthorized, verify("c@example.com", "192.0.2.9").Code, "another IP has its own bucket")
}

func jsonBody(t *testing.T, v any) io.Reader {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return bytes.NewReader(b)
}
