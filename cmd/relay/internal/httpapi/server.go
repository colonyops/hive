// Package httpapi is the relay's HTTP adapter: the /v1 API, the /in ingress,
// and the server that runs them.
package httpapi

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"time"

	"github.com/hay-kot/httpkit/errchain"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/relay/internal/config"
	"github.com/colonyops/hive/cmd/relay/internal/relay"
	"github.com/colonyops/hive/internal/web"
	"github.com/colonyops/hive/internal/web/mid"
)

const (
	VersionHeader = "Hive-Relay-Version"
	shutdownGrace = 10 * time.Second
)

type Core struct {
	Auth    *relay.Auth
	SignIn  *relay.SignIn
	Sources *relay.Sources
	Ingress *relay.Ingress
	Feed    *relay.Feed
	Store   relay.Store
	Limits  relay.Limits
	Version string
}

type api struct {
	core       Core
	ingress    *ingressHandler
	trusted    []netip.Prefix
	authLimits authLimits
	handler    http.Handler
}

func New(core Core, cfg config.Config, log zerolog.Logger) http.Handler {
	return newAPI(core, cfg, log).handler
}

func newAPI(core Core, cfg config.Config, log zerolog.Logger) *api {
	s := &api{
		core:       core,
		ingress:    newIngressHandler(core, cfg.TrustedProxies, log),
		trusted:    cfg.TrustedProxies,
		authLimits: newAuthLimits(core.Limits),
	}

	chain := errchain.New(mid.Errors(log, mapKind))
	chain.Use(retryAfter)

	mux := http.NewServeMux()
	public := func(pattern string, h errchain.HandlerFunc) {
		mux.HandleFunc(pattern, chain.ToHandlerFunc(h))
	}
	authed := func(pattern string, h errchain.HandlerFunc) {
		mux.HandleFunc(pattern, chain.ToHandlerFunc(h, s.requireBearer))
	}

	mux.HandleFunc("GET /healthz", healthz)

	public("GET /v1/auth/methods", s.authMethods)
	public("POST /v1/auth/start", s.authStart)
	public("POST /v1/auth/verify", s.authVerify)
	public("POST /v1/auth/login", s.authLogin)

	authed("GET /v1/me", s.me)
	authed("GET /v1/sources", s.listSources)
	authed("POST /v1/sources", s.createSource)
	authed("GET /v1/sources/{id}", s.getSource)
	authed("PATCH /v1/sources/{id}", s.updateSource)
	authed("DELETE /v1/sources/{id}", s.deleteSource)
	authed("POST /v1/sources/{id}/rotate", s.rotateSource)
	authed("GET /v1/sources/{id}/deliveries", s.listDeliveries)
	authed("GET /v1/tokens", s.listTokens)
	authed("POST /v1/tokens", s.createToken)
	authed("DELETE /v1/tokens/{id}", s.deleteToken)

	// The ingress token is a credential, and mid.Logger logs the raw path, so
	// /in/ bypasses it for a logger that redacts the token.
	root := http.NewServeMux()
	ingress := logIngress(log, s.ingress)
	root.Handle("/in/{token}", ingress)
	root.Handle("/in/{token}/{rest...}", ingress)
	root.Handle("/", mid.Logger(log, "/healthz")(mux))

	s.handler = versionHeader(core.Version, root)
	return s
}

func versionHeader(version string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(VersionHeader, version)
		next.ServeHTTP(w, r)
	})
}

// retryAfter sets Retry-After on the way out: a mid.Mapper sees only the
// error, not the ResponseWriter, so the header cannot be set there.
func retryAfter(next errchain.Handler) errchain.Handler {
	return errchain.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
		err := next.ServeHTTP(w, r)
		var e *relay.Error
		if errors.As(err, &e) && e.Kind == relay.KindRateLimited && e.RetryAfter > 0 {
			w.Header().Set("Retry-After", retrySeconds(e.RetryAfter))
		}
		return err
	})
}

func mapKind(err error) (int, any, bool) {
	var e *relay.Error
	if !errors.As(err, &e) {
		return 0, nil, false
	}
	status, ok := statusForKind(e.Kind)
	if !ok {
		return 0, nil, false
	}
	msg := e.Message
	if msg == "" {
		msg = string(e.Kind)
	}
	return status, web.ErrorBody{Kind: string(e.Kind), Message: msg}, true
}

func statusForKind(k relay.Kind) (int, bool) {
	switch k {
	case relay.KindNotFound:
		return http.StatusNotFound, true
	case relay.KindUnauthorized:
		return http.StatusUnauthorized, true
	case relay.KindForbidden:
		return http.StatusForbidden, true
	case relay.KindInvalid:
		return http.StatusUnprocessableEntity, true
	case relay.KindTooLarge:
		return http.StatusRequestEntityTooLarge, true
	case relay.KindRateLimited, relay.KindOverCap:
		return http.StatusTooManyRequests, true
	default:
		return 0, false
	}
}

func retrySeconds(d time.Duration) string {
	secs := int64((d + time.Second - 1) / time.Second)
	return strconv.FormatInt(max(secs, 1), 10)
}

// Serve runs h on ln until ctx is done, then gives in-flight requests
// shutdownGrace to finish.
func Serve(ctx context.Context, ln net.Listener, limits relay.Limits, h http.Handler, log zerolog.Logger) error {
	srv := &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    limits.HeaderBytes,
	}

	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}

	log.Info().Msg("shutting down http server")
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownGrace)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
