package logutils

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSlogHandlerWritesThroughZerolog(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(SlogHandler(zerolog.New(&buf).Level(zerolog.InfoLevel)))

	logger.With("window", "main").WithGroup("call").Warn("bound method failed", "method", "Read", slog.Group("args", "n", 2))
	logger.Debug("dropped below the level")

	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	require.Len(t, lines, 1)

	var got map[string]any
	require.NoError(t, json.Unmarshal(lines[0], &got))
	assert.Equal(t, "warn", got["level"])
	assert.Equal(t, "bound method failed", got["message"])
	assert.Equal(t, "main", got["window"])
	assert.Equal(t, "Read", got["call.method"])
	assert.InDelta(t, 2, got["call.args.n"], 0)
}
