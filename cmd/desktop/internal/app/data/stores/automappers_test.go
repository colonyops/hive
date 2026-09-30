package stores

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapFunc_Slice(t *testing.T) {
	toString := MapFunc[int, string](func(v int) string {
		if v == 0 {
			return "zero"
		}
		return "nonzero"
	})
	got := toString.Slice([]int{0, 1, 2})
	assert.Equal(t, []string{"zero", "nonzero", "nonzero"}, got)
}

func TestMapFunc_Slice_Empty(t *testing.T) {
	identity := MapFunc[int, int](func(v int) int { return v })
	got := identity.Slice(nil)
	assert.Empty(t, got)
}

func TestMapFunc_Err(t *testing.T) {
	double := MapFunc[int, int](func(v int) int { return v * 2 })

	got, err := double.Err(3, nil)
	require.NoError(t, err)
	assert.Equal(t, 6, got)
}

func TestMapFunc_Err_PropagatesError(t *testing.T) {
	double := MapFunc[int, int](func(v int) int { return v * 2 })
	wantErr := errors.New("row lookup failed")

	got, err := double.Err(3, wantErr)
	require.ErrorIs(t, err, wantErr)
	assert.Zero(t, got, "a propagated error returns the zero value, not a mapped one")
}

func TestMapFunc_SliceErr(t *testing.T) {
	double := MapFunc[int, int](func(v int) int { return v * 2 })

	got, err := double.SliceErr([]int{1, 2, 3}, nil)
	require.NoError(t, err)
	assert.Equal(t, []int{2, 4, 6}, got)
}

func TestMapFunc_SliceErr_PropagatesError(t *testing.T) {
	double := MapFunc[int, int](func(v int) int { return v * 2 })
	wantErr := errors.New("query failed")

	got, err := double.SliceErr([]int{1, 2, 3}, wantErr)
	require.ErrorIs(t, err, wantErr)
	assert.Nil(t, got, "a propagated error returns a nil slice rather than mapping the input")
}
