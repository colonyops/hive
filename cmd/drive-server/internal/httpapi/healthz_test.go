package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/drive-server/internal/httpapi"
	"github.com/colonyops/hive/pkg/buildinfo"
)

func TestHealthz(t *testing.T) {
	h := httpapi.New(zerolog.Nop(), buildinfo.Info{}, httpapi.Services{})

	rec := serve(h, call{method: http.MethodGet, target: "/healthz"})

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "ok", rec.Body.String())
}

func TestVersionReturnsInjectedInfo(t *testing.T) {
	h := httpapi.New(zerolog.Nop(), buildinfo.Info{Version: "1.2.3", Commit: "abc", Date: "2026-10-15"}, httpapi.Services{})

	rec := serve(h, call{method: http.MethodGet, target: "/version"})

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, map[string]string{"version": "1.2.3", "commit": "abc", "date": "2026-10-15"}, decode[map[string]string](t, rec))
}
