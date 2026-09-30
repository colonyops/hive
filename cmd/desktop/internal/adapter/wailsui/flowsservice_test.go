package wailsui

import (
	"testing"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeImagePayload(t *testing.T) {
	// "SGVsbG8=" is base64 for "Hello".
	bare, err := decodeImagePayload("SGVsbG8=")
	require.NoError(t, err)
	assert.Equal(t, "Hello", string(bare))

	prefixed, err := decodeImagePayload("data:image/png;base64,SGVsbG8=")
	require.NoError(t, err)
	assert.Equal(t, "Hello", string(prefixed))

	_, err = decodeImagePayload("not%%%base64")
	require.Error(t, err)
	assert.Equal(t, app.KindInvalid, app.KindOf(err))
}
