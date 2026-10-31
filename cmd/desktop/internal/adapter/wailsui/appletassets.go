package wailsui

import (
	"bytes"
	"net/http"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
)

// appletAssetPrefix is where an html applet's files are served. It is on the app's
// own asset handler, not the loopback server, so in the native build nothing outside
// the webview can read them. The server build refuses them: its asset handler is the
// network listener (ADR applets-are-declared-in-yaml-and-rendered-natively).
const appletAssetPrefix = "/__applet/"

type appletAssetSource interface {
	Asset(id, name string) (app.AppletAsset, error)
}

func appletAssets(applets appletAssetSource, inner application.Middleware) application.Middleware {
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
			asset, err := applets.Asset(id, name)
			if err != nil {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			// The frame's origin is opaque, so a module script or a fetch of its own file
			// is cross-origin and arrives with Origin: null. A page holds the bridge token,
			// and a frame navigated elsewhere is also null, so a page gets no CORS at all.
			if !asset.HoldsToken {
				w.Header().Set("Access-Control-Allow-Origin", "null")
			}
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Content-Type", asset.ContentType)
			http.ServeContent(w, r, asset.Name, time.Time{}, bytes.NewReader(asset.Body))
		})
	}
}
