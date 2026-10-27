package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/web"
)

func TestRecoverPanicsAnswersJSON500AndLogsTheStack(t *testing.T) {
	var logs bytes.Buffer
	h := recoverPanics(zerolog.New(&logs))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/me", nil))

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	var body web.ErrorBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, web.ErrorBody{Kind: "internal", Message: "internal error"}, body)

	var entry map[string]any
	require.NoError(t, json.Unmarshal(logs.Bytes(), &entry))
	assert.Equal(t, "boom", entry["panic"])
	assert.Equal(t, "/api/v1/me", entry["path"])
	assert.Contains(t, entry["stack"], "TestRecoverPanicsAnswersJSON500AndLogsTheStack")
}

func TestRecoverPanicsRethrowsErrAbortHandler(t *testing.T) {
	var logs bytes.Buffer
	h := recoverPanics(zerolog.New(&logs))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))

	assert.PanicsWithValue(t, http.ErrAbortHandler, func() {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	})
	assert.Empty(t, logs.String())
}

func TestReadTimeoutFitsTheLargestUpload(t *testing.T) {
	assert.Zero(t, readTimeout(0), "uncapped documents get no read timeout")
	assert.Equal(t, readTimeoutFloor+80*time.Second, readTimeout(5<<20))
}
