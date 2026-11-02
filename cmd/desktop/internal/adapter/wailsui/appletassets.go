package wailsui

import (
	"bytes"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
)

// appletAssetPrefix is where an applet's files are served. It is on the app's
// own asset handler, not the loopback server, so in the native build nothing outside
// the webview can read them. The server build refuses them: its asset handler is the
// network listener (ADR applets-are-web-pages-with-full-access-to-the-machine).
const appletAssetPrefix = "/__applet/"

// A module script or a font loads with no Referer at all, so these cross for any frame.
var appletCodeExtensions = []string{".js", ".mjs", ".css", ".woff", ".woff2", ".ttf", ".otf"}

// mayCrossToOpaqueFrame decides who can read a file. The frame's origin is opaque, so its
// own fetch arrives with Origin: null, and so does one from a foreign page the frame
// navigated to. A page holds the bridge token, so it never crosses. What a page wrote to
// its folder crosses only for that applet's own pages, told apart by Referer, which a page
// cannot set. WebKit sends the page's URL. Chromium sends an opaque frame nothing, so there
// a page cannot fetch its own data file either, and reads it with hive.readFile.
func mayCrossToOpaqueFrame(r *http.Request, id, name string) bool {
	if slices.Contains(appletCodeExtensions, strings.ToLower(path.Ext(name))) {
		return true
	}
	referer, err := url.Parse(r.Header.Get("Referer"))
	if err != nil || referer.Host == "" || referer.Host != r.Host {
		return false
	}
	return strings.HasPrefix(referer.Path, appletAssetPrefix+id+"/")
}

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
			if !asset.HoldsToken && mayCrossToOpaqueFrame(r, id, asset.Name) {
				w.Header().Set("Access-Control-Allow-Origin", "null")
			}
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Content-Type", asset.ContentType)
			http.ServeContent(w, r, asset.Name, time.Time{}, bytes.NewReader(asset.Body))
		})
	}
}
