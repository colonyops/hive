package logutils

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComponentLabelsEveryLine(t *testing.T) {
	var buf bytes.Buffer
	logger := Component(zerolog.New(&buf), "sessions")

	logger.Info().Msg("created")

	var line map[string]string
	require.NoError(t, json.Unmarshal(buf.Bytes(), &line))
	assert.Equal(t, "sessions", line["cmp"])
	assert.Equal(t, "created", line["message"])
}
