package wailsui

import (
	"bytes"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
)

// appletAssetPrefix is where an html applet's files are served. It is on the app's
// own asset handler, not the loopback server, so nothing outside this webview can
// read them.
const appletAssetPrefix = "/__applet/"

func appletAssets(applets *app.AppletsService, inner application.Middleware) application.Middleware {
	return func(next http.Handler) http.Handler {
		if inner != nil {
			next = inner(next)
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rest, ok := strings.CutPrefix(r.URL.Path, appletAssetPrefix)
			if !ok {
				next.ServeHTTP(w, r)
				return
			}
			id, name, _ := strings.Cut(rest, "/")
			if decoded, err := url.PathUnescape(name); err == nil {
				name = decoded
			}
			asset, err := applets.Asset(id, name)
			if err != nil {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			// The frame's origin is opaque, so a module script or a fetch of its own file
			// is cross-origin.
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Content-Type", asset.ContentType)
			http.ServeContent(w, r, asset.Name, time.Time{}, bytes.NewReader(asset.Body))
		})
	}
}
