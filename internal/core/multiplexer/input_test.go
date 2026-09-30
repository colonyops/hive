package multiplexer

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewNamedKey(t *testing.T) {
	key, err := NewNamedKey("C-c")
	require.NoError(t, err)
	require.Equal(t, NamedKey("C-c"), key)

	_, err = NewNamedKey("")
	require.Error(t, err)
	_, err = NewNamedKey("bad\x00key")
	require.Error(t, err)
}
