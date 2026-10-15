package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/colonyops/hive/internal/platform/driveapi"
)

const wantCSP = "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data:; font-src data:; connect-src 'none'; form-action 'none'; frame-src 'none'; base-uri 'none'; frame-ancestors 'self'; sandbox allow-scripts"

func assertRawHeaders(t *testing.T, rec *httptest.ResponseRecorder, contentType string) {
	t.Helper()
	h := rec.Header()
	assert.Equal(t, contentType, h.Get("Content-Type"))
	assert.Equal(t, wantCSP, h.Get("Content-Security-Policy"))
	assert.Equal(t, "nosniff", h.Get("X-Content-Type-Options"))
	assert.Equal(t, "noindex", h.Get("X-Robots-Tag"))
	assert.Equal(t, "no-referrer", h.Get("Referrer-Policy"))
	assert.Equal(t, "private, no-store", h.Get("Cache-Control"))
}

func share(t *testing.T, s *server, token, path, body string) (driveapi.Share, string) {
	t.Helper()
	docURL := driveapi.DocPath(drive.Shared, path)
	require.Equal(t, http.StatusOK, s.do(http.MethodPut, docURL, token, "<p>hi</p>").Code)
	rec := s.do(http.MethodPost, driveapi.SpacePrefix(drive.Shared)+"/shares/"+path, token, body)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var out driveapi.Share
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	slug, ok := strings.CutPrefix(out.URL, testBaseURL+"/s/")
	require.True(t, ok, out.URL)
	return out, slug
}

func TestRawRoutesSendTheLockedDownHeaders(t *testing.T) {
	s := newServer(t)
	token := s.token(t, "a@example.com", core.ScopePush)
	_, htmlSlug := share(t, s, token, "page.html", "")
	_, mdSlug := share(t, s, token, "note.md", "")

	rec := s.do(http.MethodGet, "/s/"+htmlSlug+"/raw", "", "")
	require.Equal(t, http.StatusOK, rec.Code)
	assertRawHeaders(t, rec, "text/html; charset=utf-8")
	assert.Equal(t, "<p>hi</p>", rec.Body.String())

	rec = s.do(http.MethodGet, "/s/"+mdSlug+"/raw", "", "")
	require.Equal(t, http.StatusOK, rec.Code)
	assertRawHeaders(t, rec, "text/markdown; charset=utf-8")

	rec = s.do(http.MethodGet, driveapi.DocPath(drive.Shared, "page.html")+"?raw=1", token, "")
	require.Equal(t, http.StatusOK, rec.Code)
	assertRawHeaders(t, rec, "text/html; charset=utf-8")

	rec = s.do(http.MethodGet, driveapi.DocPath(drive.Shared, "note.md")+"?raw=1", token, "")
	assertRawHeaders(t, rec, "text/markdown; charset=utf-8")

	rec = s.do(http.MethodGet, driveapi.DocPath(drive.Shared, "page.html")+"?raw=1", "", "")
	assert.Equal(t, http.StatusUnauthorized, rec.Code, "the signed-in route still needs a token")
}

func TestSlugRedirectsToRaw(t *testing.T) {
	s := newServer(t)
	_, slug := share(t, s, s.token(t, "a@example.com", core.ScopePush), "page.html", "")

	rec := s.do(http.MethodGet, "/s/"+slug, "", "")
	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "/s/"+slug+"/raw", rec.Header().Get("Location"))
}

func TestPublicEndpoint(t *testing.T) {
	s := newServer(t)
	token := s.token(t, "a@example.com", core.ScopePush)
	_, mdSlug := share(t, s, token, "note.md", "")
	_, htmlSlug := share(t, s, token, "page.html", "")

	rec := s.do(http.MethodGet, "/api/v1/public/"+mdSlug, "", "")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "noindex", rec.Header().Get("X-Robots-Tag"))
	var got driveapi.PublicDocument
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, "a@example.com", got.OwnerEmail)
	assert.Equal(t, "note.md", got.Document.Path)
	assert.Equal(t, "<p>hi</p>", got.Body)

	rec = s.do(http.MethodGet, "/api/v1/public/"+htmlSlug, "", "")
	var html driveapi.PublicDocument
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &html))
	assert.Empty(t, html.Body, "HTML is served from the raw route only")

	rec = s.do(http.MethodGet, "/api/v1/public/unknown", "", "")
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, "noindex", rec.Header().Get("X-Robots-Tag"))
}

func TestShareRoutesNeedPushAndValidateInput(t *testing.T) {
	s := newServer(t)
	push := s.token(t, "a@example.com", core.ScopePush)
	read := s.token(t, "a@example.com", core.ScopeRead)
	share(t, s, push, "note.md", "")
	route := "/api/v1/shared/shares/note.md"

	assert.Equal(t, http.StatusForbidden, s.do(http.MethodPost, route, read, "").Code)
	assert.Equal(t, http.StatusForbidden, s.do(http.MethodDelete, route, read, "").Code)
	assert.Equal(t, http.StatusUnauthorized, s.do(http.MethodPost, route, "", "").Code)
	assert.Equal(t, http.StatusBadRequest, s.do(http.MethodPost, route, push, "{nope").Code)
	assert.Equal(t, http.StatusUnprocessableEntity, s.do(http.MethodPost, route, push, `{"expires_in":"2d"}`).Code)
	assert.Equal(t, http.StatusOK, s.do(http.MethodPost, route, push, `{"expires_in":"7d"}`).Code)
	assert.Equal(t, http.StatusNotFound, s.do(http.MethodPost, "/api/v1/shared/shares/missing.md", push, "").Code)
}

func TestRevokeEndsTheLink(t *testing.T) {
	s := newServer(t)
	token := s.token(t, "a@example.com", core.ScopePush)
	_, slug := share(t, s, token, "note.md", "")

	assert.Equal(t, http.StatusNoContent, s.do(http.MethodDelete, "/api/v1/shared/shares/note.md", token, "").Code)
	assert.Equal(t, http.StatusNotFound, s.do(http.MethodGet, "/s/"+slug+"/raw", "", "").Code)
}

func TestListShowsShared(t *testing.T) {
	s := newServer(t)
	token := s.token(t, "a@example.com", core.ScopePush)
	out, _ := share(t, s, token, "note.md", `{"expires_in":"7d"}`)

	var docs []driveapi.Document
	require.NoError(t, json.Unmarshal(s.do(http.MethodGet, "/api/v1/shared", token, "").Body.Bytes(), &docs))
	require.Len(t, docs, 1)
	require.NotNil(t, docs[0].Shared)
	assert.Equal(t, out.URL, docs[0].Shared.URL)
	assert.NotNil(t, docs[0].Shared.ExpiresAt)
}
