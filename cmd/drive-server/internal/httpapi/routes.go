package httpapi

import (
	"net/http"

	"github.com/hay-kot/httpkit/errchain"

	"github.com/colonyops/hive/pkg/buildinfo"
)

func registerRoutes(mux *http.ServeMux, chain *errchain.ErrChain, info buildinfo.Info) {
	mux.HandleFunc("GET /healthz", healthz)
	mux.HandleFunc("GET /version", chain.ToHandlerFunc(version(info)))
}
