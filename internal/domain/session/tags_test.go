package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionHasAllTags(t *testing.T) {
	s := Session{Tags: []string{"Auto", "backend"}}
	assert.True(t, s.HasTag("auto"))
	assert.True(t, s.HasAllTags([]string{"AUTO", "backend"}))
	assert.False(t, s.HasAllTags([]string{"auto", "frontend"}))
}

func TestSessionEditTags(t *testing.T) {
	s := Session{Tags: []string{"auto", "backend"}}
	require.NoError(t, s.EditTags([]string{"orchestrated", "Backend"}, []string{"AUTO", "missing"}))
	assert.Equal(t, []string{"backend", "orchestrated"}, s.Tags)

	require.Error(t, s.EditTags([]string{"a,b"}, nil))
	assert.Equal(t, []string{"backend", "orchestrated"}, s.Tags, "a failed edit leaves the tags alone")
}
