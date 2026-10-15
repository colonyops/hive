package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/hay-kot/httpkit/errchain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/internal/web"
)

func TestMapError(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		kind   string
	}{
		{"invalid", core.NewError(core.KindInvalid, "bad"), http.StatusUnprocessableEntity, "invalid"},
		{"forbidden", core.NewError(core.KindForbidden, "no"), http.StatusForbidden, "forbidden"},
		{"not found", core.ErrNotFound, http.StatusNotFound, "not_found"},
		{"conflict", core.NewError(core.KindConflict, "dup"), http.StatusConflict, "conflict"},
		{"unauthenticated", core.NewError(core.KindUnauthenticated, "who"), http.StatusUnauthorized, "unauthenticated"},
		{"too large", core.NewError(core.KindTooLarge, "big"), http.StatusRequestEntityTooLarge, "too_large"},
		{"rate limited", core.NewError(core.KindRateLimited, "slow"), http.StatusTooManyRequests, "rate_limited"},
		{"wrapped", fmt.Errorf("op: %w", core.NewError(core.KindForbidden, "no")), http.StatusForbidden, "forbidden"},
		{"max bytes", fmt.Errorf("read: %w", &http.MaxBytesError{Limit: 10}), http.StatusRequestEntityTooLarge, "too_large"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body, ok := mapError(tt.err)
			require.True(t, ok)
			assert.Equal(t, tt.status, status)
			assert.Equal(t, tt.kind, body.(web.ErrorBody).Kind)
		})
	}
}

func TestMapErrorLeavesUnknownErrorsToTheDefault(t *testing.T) {
	_, _, ok := mapError(errors.New("boom"))
	assert.False(t, ok)
	_, _, ok = mapError(core.NewError(core.Kind("mystery"), "?"))
	assert.False(t, ok)
}

func TestRequireScope(t *testing.T) {
	var reached bool
	next := func(http.ResponseWriter, *http.Request) error { reached = true; return nil }
	run := func(scope core.Scope, have core.Scope) error {
		reached = false
		h := Require(scope)(errchain.HandlerFunc(next))
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
		require.NoError(t, err)
		return h.ServeHTTP(nil, req.WithContext(withPrincipal(req.Context(), core.Principal{Scope: have})))
	}

	require.NoError(t, run(core.ScopeRead, core.ScopeRead))
	assert.True(t, reached)
	require.NoError(t, run(core.ScopeRead, core.ScopePush))
	require.NoError(t, run(core.ScopePush, core.ScopePush))

	err := run(core.ScopePush, core.ScopeRead)
	var coreErr *core.Error
	require.ErrorAs(t, err, &coreErr)
	assert.Equal(t, core.KindForbidden, coreErr.Kind)
	assert.False(t, reached)
}
