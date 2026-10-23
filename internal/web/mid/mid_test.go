package mid

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hay-kot/httpkit/errchain"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/web"
)

func TestErrorsStatusAndKind(t *testing.T) {
	unreachable := func(error) (int, any, bool) {
		return http.StatusTeapot, web.ErrorBody{Kind: "mapper"}, true
	}
	for _, tc := range []struct {
		name   string
		err    error
		status int
		kind   string
	}{
		{"oversized body", fmt.Errorf("decode: %w", &http.MaxBytesError{Limit: 1}), http.StatusRequestEntityTooLarge, "too_large"},
		{"bad request", &web.BadRequestError{Msg: "invalid request body"}, http.StatusBadRequest, "invalid"},
		{"bad request wrapping an oversized body", &web.BadRequestError{Msg: "bad upload", Err: &http.MaxBytesError{Limit: 1}}, http.StatusBadRequest, "invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chain := errchain.New(Errors(zerolog.Nop(), unreachable))
			h := chain.ToHandlerFunc(func(http.ResponseWriter, *http.Request) error { return tc.err })

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))

			assert.Equal(t, tc.status, rec.Code)
			var body web.ErrorBody
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			assert.Equal(t, tc.kind, body.Kind)
		})
	}
}
