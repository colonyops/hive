package session

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeLabels(t *testing.T) {
	got, err := NormalizeLabels([]string{" auto ", "", "Backend", "AUTO", "backend", "needs review"})
	require.NoError(t, err)
	assert.Equal(t, []string{"auto", "Backend", "needs review"}, got)

	got, err = NormalizeLabels(nil)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestNormalizeLabelsRejectsInvalidLabels(t *testing.T) {
	for _, label := range []string{"a,b", "tab\tlabel", strings.Repeat("x", MaxLabelLength+1)} {
		_, err := NormalizeLabels([]string{label})
		require.Error(t, err, "label %q", label)
	}
	_, err := NormalizeLabels([]string{strings.Repeat("é", MaxLabelLength)})
	assert.NoError(t, err, "the limit counts characters, not bytes")
}

func TestParseLabels(t *testing.T) {
	got, err := ParseLabels("auto, backend,,Auto")
	require.NoError(t, err)
	assert.Equal(t, []string{"auto", "backend"}, got)
}

func TestSessionHasAllLabels(t *testing.T) {
	s := Session{Labels: []string{"Auto", "backend"}}
	assert.True(t, s.HasLabel("auto"))
	assert.True(t, s.HasAllLabels([]string{"AUTO", "backend"}))
	assert.True(t, s.HasAllLabels(nil))
	assert.False(t, s.HasAllLabels([]string{"auto", "frontend"}))
}

func TestSessionEditLabels(t *testing.T) {
	s := Session{Labels: []string{"auto", "backend"}}
	require.NoError(t, s.EditLabels([]string{"orchestrated", "Backend"}, []string{"AUTO", "missing"}))
	assert.Equal(t, []string{"backend", "orchestrated"}, s.Labels)

	require.Error(t, s.EditLabels([]string{"a,b"}, nil))
	assert.Equal(t, []string{"backend", "orchestrated"}, s.Labels, "a failed edit leaves the labels alone")
}
