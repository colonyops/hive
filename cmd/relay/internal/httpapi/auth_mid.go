package httpapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/hay-kot/httpkit/errchain"

	"github.com/colonyops/hive/cmd/relay/internal/relay"
)

type ctxKey int

const (
	accountKey ctxKey = iota
	tokenKey
)

// requireBearer is an errchain middleware, so a bad token is a returned
// *relay.Error that mid.Errors renders like any handler error.
func (s *api) requireBearer(next errchain.Handler) errchain.Handler {
	return errchain.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
		acct, tok, err := s.core.Auth.Authenticate(r.Context(), bearer(r))
		if err != nil {
			return err
		}
		ctx := context.WithValue(r.Context(), accountKey, acct)
		ctx = context.WithValue(ctx, tokenKey, tok)
		return next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func bearer(r *http.Request) string {
	scheme, value, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(value)
}

func accountFrom(ctx context.Context) relay.Account {
	a, _ := ctx.Value(accountKey).(relay.Account)
	return a
}

func tokenFrom(ctx context.Context) relay.Token {
	t, _ := ctx.Value(tokenKey).(relay.Token)
	return t
}
