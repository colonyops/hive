package httpapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/hay-kot/httpkit/errchain"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
)

type principalKey struct{}

func withPrincipal(ctx context.Context, p core.Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

func principalFrom(ctx context.Context) (core.Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(core.Principal)
	return p, ok
}

// Authenticate turns an "Authorization: Bearer hds_..." header, or else the
// session cookie, into a Principal in the request context. A request that
// sends a bearer header is judged on it alone, so a stale cookie cannot
// override a token.
func Authenticate(tokens *core.Tokens, sessions *core.Sessions, cookieName string) errchain.Middleware {
	return func(next errchain.Handler) errchain.Handler {
		return errchain.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
			p, err := resolvePrincipal(r, tokens, sessions, cookieName)
			if err != nil {
				return err
			}
			return next.ServeHTTP(w, r.WithContext(withPrincipal(r.Context(), p)))
		})
	}
}

func resolvePrincipal(r *http.Request, tokens *core.Tokens, sessions *core.Sessions, cookieName string) (core.Principal, error) {
	if secret := bearer(r); secret != "" {
		return tokens.Resolve(r.Context(), secret)
	}
	if c, err := r.Cookie(cookieName); err == nil && c.Value != "" {
		return sessions.Resolve(r.Context(), c.Value)
	}
	return core.Principal{}, core.NewError(core.KindUnauthenticated, "missing token")
}

// RequireSession rejects a request that did not authenticate by session
// cookie, so a token cannot mint more tokens. It runs after Authenticate.
func RequireSession() errchain.Middleware {
	return func(next errchain.Handler) errchain.Handler {
		return errchain.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
			p, ok := principalFrom(r.Context())
			if !ok {
				return core.NewError(core.KindUnauthenticated, "not signed in")
			}
			if !p.ViaSession {
				return core.NewError(core.KindForbidden, "this request needs a signed-in browser session, not a token")
			}
			return next.ServeHTTP(w, r)
		})
	}
}

// Require rejects a request whose principal lacks scope. It runs after
// Authenticate.
func Require(scope core.Scope) errchain.Middleware {
	return func(next errchain.Handler) errchain.Handler {
		return errchain.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
			p, ok := principalFrom(r.Context())
			if !ok {
				return core.NewError(core.KindUnauthenticated, "not signed in")
			}
			if !p.Scope.Covers(scope) {
				return core.NewError(core.KindForbidden, "token scope does not allow this request")
			}
			return next.ServeHTTP(w, r)
		})
	}
}

func bearer(r *http.Request) string {
	scheme, value, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(value)
}
