package httpapi

import (
	"net/http"
	"strings"

	"github.com/hay-kot/httpkit/errchain"
	"github.com/hay-kot/httpkit/server"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/relay/internal/authctx"
	"github.com/colonyops/hive/internal/web"
)

// requireBearer is an errchain middleware, so a bad token is a returned
// *relay.Error that mid.Errors renders like any handler error.
func (s *api) requireBearer(next errchain.Handler) errchain.Handler {
	return errchain.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
		acct, tok, err := s.core.Auth.Authenticate(r.Context(), bearer(r))
		if err != nil {
			return err
		}
		return next.ServeHTTP(w, r.WithContext(authctx.WithAccount(r.Context(), acct, tok)))
	})
}

// requireBearerPlain guards a plain http.Handler, the MCP server, which
// errchain cannot wrap, and so renders its own error body.
func (s *api) requireBearerPlain(log zerolog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		acct, tok, err := s.core.Auth.Authenticate(r.Context(), bearer(r))
		if err == nil {
			next.ServeHTTP(w, r.WithContext(authctx.WithAccount(r.Context(), acct, tok)))
			return
		}
		status, body, ok := mapKind(err)
		if !ok {
			log.Error().Err(err).Msg("authenticate mcp request")
			status, body = http.StatusInternalServerError, web.ErrorBody{Kind: "internal", Message: "internal error"}
		}
		if status == http.StatusUnauthorized {
			w.Header().Set("WWW-Authenticate", "Bearer")
		}
		if err := server.JSON(w, status, body); err != nil {
			log.Debug().Err(err).Msg("write mcp auth error")
		}
	})
}

func bearer(r *http.Request) string {
	scheme, value, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(value)
}
