package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

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
