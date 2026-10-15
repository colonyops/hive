package httpapi

import (
	"net/http"

	"github.com/hay-kot/httpkit/errchain"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/pkg/buildinfo"
)

func registerRoutes(mux *http.ServeMux, chain *errchain.ErrChain, info buildinfo.Info, svc Services) {
	cookies := newCookies(svc.BaseURL)
	authenticated := func(scope core.Scope, h errchain.HandlerFunc) http.HandlerFunc {
		return chain.ToHandlerFunc(h, Authenticate(svc.Tokens, svc.Sessions, cookies.name), Require(scope))
	}
	sessionOnly := func(h errchain.HandlerFunc) http.HandlerFunc {
		return chain.ToHandlerFunc(h, Authenticate(svc.Tokens, svc.Sessions, cookies.name), RequireSession())
	}

	mux.HandleFunc("GET /healthz", healthz)
	mux.HandleFunc("GET /version", chain.ToHandlerFunc(version(info)))
	mux.HandleFunc("GET /api/v1/me", authenticated(core.ScopeRead, me(svc.Documents)))
	registerDocumentRoutes(mux, authenticated, svc.Documents)
	registerShareRoutes(mux, chain, authenticated, svc.Shares)
	registerAuthRoutes(mux, chain, sessionOnly, svc, cookies)
	registerTokenRoutes(mux, sessionOnly, svc.Tokens)
	if svc.Web != nil {
		page := spa(svc.Web)
		mux.HandleFunc("GET /", chain.ToHandlerFunc(page))
		// Specific so that the share page carries noindex; the SPA reads the link itself.
		mux.HandleFunc("GET /s/{slug}", chain.ToHandlerFunc(noindex(page)))
	}
}
