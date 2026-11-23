package tagset

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalize(t *testing.T) {
	got, err := Normalize([]string{" auto ", "", "Backend", "AUTO", "backend", "needs review"})
	require.NoError(t, err)
	assert.Equal(t, []string{"auto", "Backend", "needs review"}, got)

	got, err = Normalize(nil)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestNormalizeRejectsInvalidTags(t *testing.T) {
	for _, tag := range []string{"a,b", "tab\ttag", strings.Repeat("x", MaxLength+1)} {
		_, err := Normalize([]string{tag})
		require.ErrorIs(t, err, ErrInvalid, "tag %q", tag)
	}
	_, err := Normalize([]string{strings.Repeat("é", MaxLength)})
	assert.NoError(t, err, "the limit counts characters, not bytes")
}

func TestParse(t *testing.T) {
	got, err := Parse("auto, backend,,Auto")
	require.NoError(t, err)
	assert.Equal(t, []string{"auto", "backend"}, got)
}

func TestContainsAll(t *testing.T) {
	tags := []string{"Auto", "backend"}
	assert.True(t, Contains(tags, " auto "))
	assert.True(t, ContainsAll(tags, []string{"AUTO", "backend"}))
	assert.True(t, ContainsAll(tags, nil))
	assert.False(t, ContainsAll(tags, []string{"auto", "frontend"}))
}

func TestEdit(t *testing.T) {
	tags := []string{"auto", "backend"}
	got, err := Edit(tags, []string{"orchestrated", "Backend"}, []string{"AUTO", "missing"})
	require.NoError(t, err)
	assert.Equal(t, []string{"backend", "orchestrated"}, got)
	assert.Equal(t, []string{"auto", "backend"}, tags, "the input is not changed")

	_, err = Edit(tags, []string{"a,b"}, nil)
	require.ErrorIs(t, err, ErrInvalid)
}

func TestEditRemovesATagStoredBeforeValidation(t *testing.T) {
	legacy := strings.Repeat("x", MaxLength+1)
	got, err := Edit([]string{"auto", legacy}, []string{"claimed"}, []string{legacy})
	require.NoError(t, err)
	assert.Equal(t, []string{"auto", "claimed"}, got)
}
