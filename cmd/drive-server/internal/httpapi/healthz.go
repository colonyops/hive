package httpapi

import (
	"io"
	"net/http"

	"github.com/hay-kot/httpkit/errchain"
	"github.com/hay-kot/httpkit/server"

	"github.com/colonyops/hive/pkg/buildinfo"
)

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, "ok")
}

// version serves the build identity under lowercase keys, which
// buildinfo.Info does not tag.
func version(info buildinfo.Info) errchain.HandlerFunc {
	body := struct {
		Version string `json:"version"`
		Commit  string `json:"commit"`
		Date    string `json:"date"`
	}(info)
	return func(w http.ResponseWriter, _ *http.Request) error {
		return server.JSON(w, http.StatusOK, body)
	}
}
