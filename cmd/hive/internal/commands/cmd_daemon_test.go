package commands

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func logMessages(t *testing.T, out *bytes.Buffer) []string {
	t.Helper()
	var msgs []string
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var entry struct{ Message string }
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		msgs = append(msgs, entry.Message)
	}
	return msgs
}

func TestLogWriterKeepsAcceptingWritesAfterAnOverlongLine(t *testing.T) {
	var out bytes.Buffer
	w := newLogWriter(zerolog.New(&out), zerolog.InfoLevel)

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = w.Write([]byte(strings.Repeat("x", 2*maxLogLine) + "\n"))
		// A redraw that never ends a line must not buffer without bound.
		_, _ = w.Write([]byte(strings.Repeat("y", maxLogLine+10)))
		_, _ = w.Write([]byte("\nshort\n"))
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("a write after an overlong line blocked")
	}

	msgs := logMessages(t, &out)
	lengths := make([]int, len(msgs))
	for i, m := range msgs {
		lengths[i] = len(m)
	}
	assert.Equal(t, []int{2 * maxLogLine, maxLogLine, 10, 5}, lengths)
	assert.Equal(t, "short", msgs[len(msgs)-1])
}

func TestLogWriterJoinsPartialWritesIntoLines(t *testing.T) {
	var out bytes.Buffer
	w := newLogWriter(zerolog.New(&out), zerolog.InfoLevel)
	_, _ = w.Write([]byte("hel"))
	_, _ = w.Write([]byte("lo\r\nwor"))
	_, _ = w.Write([]byte("ld\n"))
	assert.Equal(t, []string{"hello", "world"}, logMessages(t, &out))
}
