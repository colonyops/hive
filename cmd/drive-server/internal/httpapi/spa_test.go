package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/pkg/buildinfo"
)

func getSPA(t *testing.T, web fstest.MapFS, target string) *httptest.ResponseRecorder {
	t.Helper()
	h := New(zerolog.Nop(), buildinfo.Info{}, Services{Web: web})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

var builtUI = fstest.MapFS{
	"index.html":      {Data: []byte("<html>app</html>")},
	"assets/app-1.js": {Data: []byte("console.log(1)")},
}

func TestSPAFallsBackToIndexOnDeepLink(t *testing.T) {
	rec := getSPA(t, builtUI, "/d/notes/a.md")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "<html>app</html>", rec.Body.String())
	assert.Equal(t, "no-cache", rec.Header().Get("Cache-Control"))
	assert.Equal(t, pageCSP, rec.Header().Get("Content-Security-Policy"))
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "same-origin", rec.Header().Get("Referrer-Policy"))
}

func TestSPAAssetIsImmutable(t *testing.T) {
	rec := getSPA(t, builtUI, "/assets/app-1.js")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "console.log(1)", rec.Body.String())
	assert.Equal(t, "public, max-age=31536000, immutable", rec.Header().Get("Cache-Control"))
	assert.Equal(t, pageCSP, rec.Header().Get("Content-Security-Policy"))
}

func TestSPAUnknownAPIPathIsJSON404(t *testing.T) {
	rec := getSPA(t, builtUI, "/api/unknown")

	require.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "not_found", body["kind"])
}

func TestSPAWithoutIndexIs503(t *testing.T) {
	rec := getSPA(t, fstest.MapFS{".gitkeep": {}}, "/")

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Contains(t, rec.Body.String(), "web UI not built")
}

func TestSPAShareLinkPageIsNoindex(t *testing.T) {
	rec := getSPA(t, builtUI, "/s/abc123")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "<html>app</html>", rec.Body.String())
	assert.Equal(t, "noindex", rec.Header().Get("X-Robots-Tag"))
	assert.Equal(t, pageCSP, rec.Header().Get("Content-Security-Policy"))
}
