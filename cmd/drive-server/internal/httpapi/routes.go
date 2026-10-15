package httpapi

import (
	"net/http"

	"github.com/hay-kot/httpkit/errchain"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/pkg/buildinfo"
)

func registerRoutes(mux *http.ServeMux, chain *errchain.ErrChain, info buildinfo.Info, svc Services) {
	authenticated := func(scope core.Scope, h errchain.HandlerFunc) http.HandlerFunc {
		return chain.ToHandlerFunc(h, Authenticate(svc.Tokens), Require(scope))
	}

	mux.HandleFunc("GET /healthz", healthz)
	mux.HandleFunc("GET /version", chain.ToHandlerFunc(version(info)))
	mux.HandleFunc("GET /api/v1/me", authenticated(core.ScopeRead, me(svc.Documents)))
	registerDocumentRoutes(mux, authenticated, svc.Documents)
}
