package httpapi

import (
	"bytes"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/hay-kot/httpkit/errchain"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
)

const pageCSP = "default-src 'self'; script-src 'self' 'wasm-unsafe-eval'; img-src 'self' data:; " +
	"style-src 'self' 'unsafe-inline'; frame-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'"

const indexFile = "index.html"

// spa serves the built web UI from web. Unknown paths fall back to index.html
// so client-side routes survive a reload; unknown /api/ paths do not, because a
// client must get a JSON error there.
func spa(web fs.FS) errchain.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			return core.NewError(core.KindNotFound, "no such endpoint")
		}

		h := w.Header()
		h.Set("Content-Security-Policy", pageCSP)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")

		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name == "" || !isFile(web, name) {
			name = indexFile
		}

		data, err := fs.ReadFile(web, name)
		if err != nil {
			// A Go-only build embeds no index.html.
			http.Error(w, "web UI not built", http.StatusServiceUnavailable)
			return nil
		}

		switch {
		case strings.HasPrefix(name, "assets/"):
			h.Set("Cache-Control", "public, max-age=31536000, immutable")
		case name == indexFile:
			h.Set("Cache-Control", "no-cache")
		}
		http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
		return nil
	}
}

func isFile(fsys fs.FS, name string) bool {
	f, err := fsys.Open(name)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	return err == nil && info.Mode().IsRegular()
}
