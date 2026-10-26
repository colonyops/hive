package session

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeTags(t *testing.T) {
	got, err := NormalizeTags([]string{" auto ", "", "Backend", "AUTO", "backend", "needs review"})
	require.NoError(t, err)
	assert.Equal(t, []string{"auto", "Backend", "needs review"}, got)

	got, err = NormalizeTags(nil)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestNormalizeTagsRejectsInvalidTags(t *testing.T) {
	for _, tag := range []string{"a,b", "tab\ttag", strings.Repeat("x", MaxTagLength+1)} {
		_, err := NormalizeTags([]string{tag})
		require.Error(t, err, "tag %q", tag)
	}
	_, err := NormalizeTags([]string{strings.Repeat("é", MaxTagLength)})
	assert.NoError(t, err, "the limit counts characters, not bytes")
}

func TestParseTags(t *testing.T) {
	got, err := ParseTags("auto, backend,,Auto")
	require.NoError(t, err)
	assert.Equal(t, []string{"auto", "backend"}, got)
}

func TestSessionHasAllTags(t *testing.T) {
	s := Session{Tags: []string{"Auto", "backend"}}
	assert.True(t, s.HasTag("auto"))
	assert.True(t, s.HasAllTags([]string{"AUTO", "backend"}))
	assert.True(t, s.HasAllTags(nil))
	assert.False(t, s.HasAllTags([]string{"auto", "frontend"}))
}

func TestSessionEditTags(t *testing.T) {
	s := Session{Tags: []string{"auto", "backend"}}
	require.NoError(t, s.EditTags([]string{"orchestrated", "Backend"}, []string{"AUTO", "missing"}))
	assert.Equal(t, []string{"backend", "orchestrated"}, s.Tags)

	require.Error(t, s.EditTags([]string{"a,b"}, nil))
	assert.Equal(t, []string{"backend", "orchestrated"}, s.Tags, "a failed edit leaves the tags alone")
}
