package extractors

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/web"
)

type scalarQuery struct {
	Name    string `query:"name"`
	Limit   int    `query:"limit"`
	Offset  int64  `query:"offset"`
	Verbose bool   `query:"verbose"`
	Skipped string
	Dash    string `query:"-"`
}

type sliceQuery struct {
	Tags []string `query:"tag"`
	IDs  []int    `query:"id"`
	Big  []int64  `query:"big"`
	Flag []bool   `query:"flag"`
}

type validatedQuery struct {
	Limit int `query:"limit"`
}

var errLimit = errors.New("limit too high")

func (q validatedQuery) Validate() error {
	if q.Limit > 10 {
		return errLimit
	}
	return nil
}

func queryRequest(rawQuery string) *http.Request {
	return httptest.NewRequest(http.MethodGet, "/x?"+rawQuery, nil)
}

func TestQuery_ScalarTypes(t *testing.T) {
	got, err := Query[scalarQuery](queryRequest("name=ada&limit=5&offset=9000000000&verbose=true"))
	require.NoError(t, err)
	assert.Equal(t, scalarQuery{Name: "ada", Limit: 5, Offset: 9000000000, Verbose: true}, got)
}

func TestQuery_NonSliceTakesFirstValue(t *testing.T) {
	got, err := Query[scalarQuery](queryRequest("name=a&name=b"))
	require.NoError(t, err)
	assert.Equal(t, "a", got.Name)
}

func TestQuery_Slices(t *testing.T) {
	got, err := Query[sliceQuery](queryRequest("tag=a&tag=b&id=1&id=2&big=3&flag=true&flag=false"))
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, got.Tags)
	assert.Equal(t, []int{1, 2}, got.IDs)
	assert.Equal(t, []int64{3}, got.Big)
	assert.Equal(t, []bool{true, false}, got.Flag)
}

func TestQuery_MissingKeysLeaveZeroValues(t *testing.T) {
	got, err := Query[sliceQuery](queryRequest(""))
	require.NoError(t, err)
	assert.Nil(t, got.Tags)
	assert.Nil(t, got.IDs)

	scalars, err := Query[scalarQuery](queryRequest(""))
	require.NoError(t, err)
	assert.Equal(t, scalarQuery{}, scalars)
}

func TestQuery_UntaggedAndDashFieldsSkipped(t *testing.T) {
	got, err := Query[scalarQuery](queryRequest("Skipped=x&-=y&Dash=z"))
	require.NoError(t, err)
	assert.Empty(t, got.Skipped)
	assert.Empty(t, got.Dash)
}

func TestQuery_BadValues(t *testing.T) {
	tests := []struct {
		name  string
		query string
		msg   string
	}{
		{"bad int", "limit=abc", "invalid query parameter limit"},
		{"bad bool", "verbose=maybe", "invalid query parameter verbose"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Query[scalarQuery](queryRequest(tt.query))
			var bad *web.BadRequestError
			require.ErrorAs(t, err, &bad)
			assert.Equal(t, tt.msg, bad.Msg)
			assert.Error(t, bad.Err)
		})
	}
}

func TestQuery_BadSliceElement(t *testing.T) {
	_, err := Query[sliceQuery](queryRequest("id=1&id=x"))
	var bad *web.BadRequestError
	require.ErrorAs(t, err, &bad)
	assert.Equal(t, "invalid query parameter id", bad.Msg)
}

func TestQuery_Validate(t *testing.T) {
	got, err := Query[validatedQuery](queryRequest("limit=5"))
	require.NoError(t, err)
	assert.Equal(t, 5, got.Limit)

	_, err = Query[validatedQuery](queryRequest("limit=50"))
	require.ErrorIs(t, err, errLimit)
}
