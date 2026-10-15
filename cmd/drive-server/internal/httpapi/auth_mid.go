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

// Authenticate turns an "Authorization: Bearer hds_..." header into a
// Principal in the request context.
func Authenticate(tokens *core.Tokens) errchain.Middleware {
	return func(next errchain.Handler) errchain.Handler {
		return errchain.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
			secret := bearer(r)
			if secret == "" {
				return core.NewError(core.KindUnauthenticated, "missing token")
			}
			p, err := tokens.Resolve(r.Context(), secret)
			if err != nil {
				return err
			}
			return next.ServeHTTP(w, r.WithContext(withPrincipal(r.Context(), p)))
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
