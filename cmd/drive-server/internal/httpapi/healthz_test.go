package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/pkg/buildinfo"
)

func TestHealthz(t *testing.T) {
	h := New(zerolog.Nop(), buildinfo.Info{}, Services{})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "ok", rec.Body.String())
}

func TestVersionReturnsInjectedInfo(t *testing.T) {
	h := New(zerolog.Nop(), buildinfo.Info{Version: "1.2.3", Commit: "abc", Date: "2026-10-15"}, Services{})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/version", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	var got map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, map[string]string{"version": "1.2.3", "commit": "abc", "date": "2026-10-15"}, got)
}
