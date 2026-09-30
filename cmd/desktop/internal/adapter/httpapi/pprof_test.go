package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPprofHandlerUsesCompatibleCPUHandler(t *testing.T) {
	cpu := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Test-CPU-Handler", "compatible")
		w.WriteHeader(http.StatusAccepted)
	})
	handler := PprofHandler(cpu)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/debug/pprof/profile?seconds=1", nil))

	assert.Equal(t, http.StatusAccepted, rec.Code)
	assert.Equal(t, "compatible", rec.Header().Get("X-Test-CPU-Handler"))
}

func TestPprofHandlerRetainsStandardIndex(t *testing.T) {
	handler := PprofHandler(nil)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Types of profiles available")
}
