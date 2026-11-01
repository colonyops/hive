package wailsui

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
)

type fakeAppletAssets map[string]app.AppletAsset

func (f fakeAppletAssets) Asset(id, name string) (app.AppletAsset, error) {
	if asset, ok := f[id+"/"+name]; ok {
		return asset, nil
	}
	return app.AppletAsset{}, errors.New("no such file")
}

func serveApplet(t *testing.T, path string) *httptest.ResponseRecorder {
	t.Helper()
	source := fakeAppletAssets{
		"pong/index.html": {Name: "index.html", ContentType: "text/html", Body: []byte("page"), HoldsToken: true},
		"pong/game.js":    {Name: "game.js", ContentType: "text/javascript", Body: []byte("code")},
		"pong/a%20b.js":   {Name: "a%20b.js", ContentType: "text/javascript", Body: []byte("literal")},
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	inner := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Inner", "yes")
			h.ServeHTTP(w, r)
		})
	}
	recorder := httptest.NewRecorder()
	appletAssets(source, inner)(next).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	return recorder
}

func TestAppletAssetsLeaveOtherPathsToTheChain(t *testing.T) {
	response := serveApplet(t, "/assets/app.js")
	assert.Equal(t, http.StatusTeapot, response.Code)
	assert.Equal(t, "yes", response.Header().Get("X-Inner"))
}

func TestAppletAssetsGiveAPageNoCORS(t *testing.T) {
	response := serveApplet(t, "/__applet/pong/index.html")
	require.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "page", response.Body.String())
	assert.Empty(t, response.Header().Get("Access-Control-Allow-Origin"), "a page holds the bridge token")
	assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
}

func TestAppletAssetsLetTheOpaqueFrameLoadItsModules(t *testing.T) {
	response := serveApplet(t, "/__applet/pong/game.js")
	require.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "null", response.Header().Get("Access-Control-Allow-Origin"))
}

func TestAppletAssetsDecodeThePathOnce(t *testing.T) {
	response := serveApplet(t, "/__applet/pong/a%2520b.js")
	require.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "literal", response.Body.String())
}

func TestAppletAssetsReportAMissingFile(t *testing.T) {
	response := serveApplet(t, "/__applet/pong/absent.js")
	assert.Equal(t, http.StatusNotFound, response.Code)
	assert.Empty(t, response.Header().Get("Access-Control-Allow-Origin"))
}

func TestAppletDataCrossesOnlyToThatAppletsOwnPages(t *testing.T) {
	source := fakeAppletAssets{
		"pong/config.json": {Name: "config.json", ContentType: "application/json", Body: []byte("{}")},
		"pong/game.MJS":    {Name: "game.MJS", ContentType: "text/javascript", Body: []byte("code")},
	}
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	tests := []struct {
		name    string
		path    string
		referer string
		want    string
	}{
		{"its own page", "/__applet/pong/config.json", "wails://example.com/__applet/pong/index.html", "null"},
		{"only the origin", "/__applet/pong/config.json", "http://example.com/", ""},
		{"a foreign page", "/__applet/pong/config.json", "http://127.0.0.1:50399/evil.html", ""},
		{"a foreign page with a matching path", "/__applet/pong/config.json", "https://evil.example/__applet/pong/x.html", ""},
		{"another applet", "/__applet/pong/config.json", "wails://example.com/__applet/other/index.html", ""},
		{"a similar applet id", "/__applet/pong/config.json", "wails://example.com/__applet/pong-2/index.html", ""},
		{"the app itself by a path", "/__applet/pong/config.json", "wails://example.com/settings", ""},
		{"no referer", "/__applet/pong/config.json", "", ""},
		{"a module, which sends no referer", "/__applet/pong/game.MJS", "", "null"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if tt.referer != "" {
				request.Header.Set("Referer", tt.referer)
			}
			recorder := httptest.NewRecorder()
			appletAssets(source, nil)(next).ServeHTTP(recorder, request)
			require.Equal(t, http.StatusOK, recorder.Code)
			assert.Equal(t, tt.want, recorder.Header().Get("Access-Control-Allow-Origin"))
		})
	}
}
