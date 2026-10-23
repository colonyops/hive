package extractors

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/web"
)

type named struct {
	Name string `json:"name"`
}

func TestBodySeparatesOversizedFromMalformed(t *testing.T) {
	decode := func(body string) error {
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		_, err := Body[named](httptest.NewRecorder(), r)
		return err
	}

	err := decode(`{"name":"` + strings.Repeat("x", maxBodyBytes) + `"}`)
	tooLarge, ok := errors.AsType[*http.MaxBytesError](err)
	require.True(t, ok, err)
	assert.EqualValues(t, maxBodyBytes, tooLarge.Limit)
	_, isBadReq := errors.AsType[*web.BadRequestError](err)
	assert.False(t, isBadReq)

	_, isBadReq = errors.AsType[*web.BadRequestError](decode(`{nope`))
	assert.True(t, isBadReq)
}
