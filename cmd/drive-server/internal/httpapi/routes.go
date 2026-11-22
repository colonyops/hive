package httpapi

import (
	"net/http"

	"github.com/hay-kot/httpkit/errchain"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/pkg/buildinfo"
)

// router is the part of *http.ServeMux that registration uses, so a test can
// record every pattern.
type router interface {
	HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request))
}

func registerRoutes(mux router, chain *errchain.ErrChain, log zerolog.Logger, info buildinfo.Info, svc Services) {
	cookies := newCookies(svc.BaseURL)
	authenticated := func(scope core.Scope, h errchain.HandlerFunc) http.HandlerFunc {
		return chain.ToHandlerFunc(h, Authenticate(svc.Tokens, svc.Sessions, cookies.name), Require(scope))
	}
	sessionOnly := func(h errchain.HandlerFunc) http.HandlerFunc {
		return chain.ToHandlerFunc(h, Authenticate(svc.Tokens, svc.Sessions, cookies.name), RequireSession())
	}

	viewer := optionalPrincipal(svc.Tokens, svc.Sessions, cookies.name)

	mux.HandleFunc("GET /healthz", healthz)
	mux.HandleFunc("GET /version", chain.ToHandlerFunc(version(info)))
	registerMeRoutes(mux, authenticated, sessionOnly, meHandlers{docs: svc.Documents, accounts: svc.Accounts})
	registerDocumentRoutes(mux, authenticated, svc.Documents)
	registerShareRoutes(mux, chain, authenticated, svc.Shares)
	registerCommentRoutes(mux, chain, authenticated, viewer, svc.Comments)
	registerLiveRoutes(mux, chain, authenticated, viewer, liveHandlers{
		log:      log,
		hub:      svc.Live,
		comments: svc.Comments,
		accounts: svc.Accounts,
		trusted:  svc.TrustedProxies,
	})
	registerAuthRoutes(mux, chain, sessionOnly, svc, cookies)
	registerTokenRoutes(mux, sessionOnly, svc.Tokens)
	if svc.Web != nil {
		page := spa(svc.Web)
		mux.HandleFunc("GET /", chain.ToHandlerFunc(page))
		// Specific so that the share page carries noindex; the SPA reads the link itself.
		mux.HandleFunc("GET /s/{slug}", chain.ToHandlerFunc(noindex(page)))
	}
}
