package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/relay/internal/config"
	"github.com/colonyops/hive/cmd/relay/internal/relay"
)

const owner = "owner@example.com"

func TestIngressUnknownToken(t *testing.T) {
	h := newHarness(t, relay.DefaultLimits())
	rec := h.do(ingressPost("nope", "x"))
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, testVersion, rec.Header().Get(VersionHeader))
}

func TestIngressWrongSecret(t *testing.T) {
	h := newHarness(t, relay.DefaultLimits())
	_, token := h.source(owner, "s3cret")

	tests := []struct {
		name   string
		header string
		value  string
		want   int
	}{
		{"no secret", "", "", http.StatusUnauthorized},
		{"wrong X-Hive-Secret", "X-Hive-Secret", "nope", http.StatusUnauthorized},
		{"wrong bearer", "Authorization", "Bearer nope", http.StatusUnauthorized},
		{"right X-Hive-Secret", "X-Hive-Secret", "s3cret", http.StatusOK},
		{"right X-Gotify-Key", "X-Gotify-Key", "s3cret", http.StatusOK},
		{"right bearer", "Authorization", "Bearer s3cret", http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := ingressPost(token, "x")
			if tt.header != "" {
				r.Header.Set(tt.header, tt.value)
			}
			assert.Equal(t, tt.want, h.do(r).Code)
		})
	}
}

type countingReader struct {
	r io.Reader
	n int
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += n
	return n, err
}

func TestIngressDeclaredLengthOverCapReadsNothing(t *testing.T) {
	limits := relay.DefaultLimits()
	limits.BodyBytes = 16
	h := newHarness(t, limits)
	_, token := h.source(owner, "")

	body := &countingReader{r: strings.NewReader(strings.Repeat("a", 17))}
	r := newRequest(http.MethodPost, "/in/"+token, body)
	r.ContentLength = 17

	rec := h.do(r)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	assert.Zero(t, body.n)
}

func TestIngressChunkedBodyOverCap(t *testing.T) {
	limits := relay.DefaultLimits()
	limits.BodyBytes = 16
	h := newHarness(t, limits)
	src, token := h.source(owner, "")

	r := newRequest(http.MethodPost, "/in/"+token, io.NopCloser(strings.NewReader(strings.Repeat("a", 17))))
	r.ContentLength = -1

	assert.Equal(t, http.StatusRequestEntityTooLarge, h.do(r).Code)
	page, err := h.core.Feed.ListAfter(context.Background(), owner, src.ID, "", 10)
	require.NoError(t, err)
	assert.Empty(t, page.Deliveries)
}

func TestIngressPerSourceRateLimit(t *testing.T) {
	limits := relay.DefaultLimits()
	limits.IngressPerSource = relay.Rate{Per: time.Hour, Count: 1, Burst: 2}
	h := newHarness(t, limits)
	_, token := h.source(owner, "")

	for range 2 {
		require.Equal(t, http.StatusOK, h.do(ingressPost(token, "x")).Code)
	}
	rec := h.do(ingressPost(token, "x"))
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("Retry-After"))
}

func TestIngressSemaphoreFull(t *testing.T) {
	limits := relay.DefaultLimits()
	limits.Concurrency = 1
	h := newHarness(t, limits)
	_, token := h.source(owner, "")

	release, ok := h.api.ingress.sem.TryAcquire()
	require.True(t, ok)
	rec := h.do(ingressPost(token, "x"))
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Equal(t, "1", rec.Header().Get("Retry-After"))

	release()
	assert.Equal(t, http.StatusOK, h.do(ingressPost(token, "x")).Code)
}

func TestIngressOverCap(t *testing.T) {
	limits := relay.DefaultLimits()
	limits.BytesPerAccount = 400
	h := newHarness(t, limits)
	_, token := h.source(owner, "")

	var rec *httptest.ResponseRecorder
	for range 10 {
		rec = h.do(ingressPost(token, strings.Repeat("a", 100)))
		if rec.Code != http.StatusOK {
			break
		}
	}
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Empty(t, rec.Header().Get("Retry-After"))
	assert.True(t, strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain"))
}

func TestIngressStoresBodyAsDelivered(t *testing.T) {
	h := newHarness(t, relay.DefaultLimits())
	src, token := h.source(owner, "")

	r := newRequest(http.MethodPost, "/in/"+token+"/hook/a?x=1", strings.NewReader(`{"a":`))
	r.Header.Set("Content-Type", "application/json")
	rec := h.do(r)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, testVersion, rec.Header().Get(VersionHeader))

	var resp ingressResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NotEmpty(t, resp.ID)

	page, err := h.core.Feed.ListAfter(context.Background(), owner, src.ID, "", 10)
	require.NoError(t, err)
	require.Len(t, page.Deliveries, 1)
	d := page.Deliveries[0]
	assert.Equal(t, resp.ID, d.ID)
	assert.Equal(t, `{"a":`, d.Body)
	assert.Equal(t, "application/json", d.ContentType)
	assert.Equal(t, http.MethodPost, d.Method)
	assert.Equal(t, "/hook/a", d.Path)
	assert.Equal(t, "1", d.Query.Get("x"))
	assert.Equal(t, int64(5), d.ContentLength)
}

func TestIngressIgnoresForwardedForWithoutTrustedProxies(t *testing.T) {
	limits := relay.DefaultLimits()
	limits.IngressPerIP = relay.Rate{Per: time.Hour, Count: 1, Burst: 1}
	h := newHarness(t, limits)
	_, token := h.source(owner, "")

	first := ingressPost(token, "x")
	first.Header.Set("X-Forwarded-For", "198.51.100.1")
	require.Equal(t, http.StatusOK, h.do(first).Code)

	second := ingressPost(token, "x")
	second.Header.Set("X-Forwarded-For", "198.51.100.2")
	assert.Equal(t, http.StatusTooManyRequests, h.do(second).Code)
}

func TestIngressLogRedactsToken(t *testing.T) {
	var buf bytes.Buffer
	h := newHarness(t, relay.DefaultLimits())
	h.api = newAPI(h.core, config.Config{Limits: h.core.Limits}, zerolog.New(&buf))
	_, token := h.source(owner, "")

	require.Equal(t, http.StatusOK, h.do(ingressPost(token, "x")).Code)
	require.Equal(t, http.StatusNotFound, h.do(ingressPost("unknown-token/a/b", "x")).Code)

	assert.NotContains(t, buf.String(), token)
	assert.NotContains(t, buf.String(), "unknown-token")
	assert.Contains(t, buf.String(), `"path":"/in/<token>/a/b"`)
}
