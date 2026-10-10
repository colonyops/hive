package app

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

func clientLogLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var lines []map[string]any
	for raw := range bytes.SplitSeq(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
		if len(raw) == 0 {
			continue
		}
		var line map[string]any
		require.NoError(t, json.Unmarshal(raw, &line))
		lines = append(lines, line)
	}
	return lines
}

func TestClientLogWritesEntriesThroughTheRootLogger(t *testing.T) {
	var buf bytes.Buffer
	svc := newClientLogService(zerolog.New(&buf).Level(zerolog.DebugLevel))
	at := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)

	svc.Write(t.Context(), []ClientLogEntry{{
		At:      at,
		Level:   "error",
		Name:    "Feed-State",
		Message: "Unable to load flows",
		Error:   "boom",
		Kind:    "internal",
		Stack:   "Error: boom\n    at load (feed.ts:1:1)",
		Window:  "main",
		Route:   "#/inbox",
		Count:   3,
		Fields:  map[string]any{"flowId": "f1", "cmp": "dirwatch", "level": "debug"},
	}})

	lines := clientLogLines(t, &buf)
	require.Len(t, lines, 1)
	line := lines[0]
	assert.Equal(t, "error", line["level"])
	assert.Equal(t, "webview.feed-state", line["cmp"])
	assert.Equal(t, "Unable to load flows", line["message"])
	assert.Equal(t, "boom", line["error"])
	assert.Equal(t, "internal", line["kind"])
	assert.Equal(t, "main", line["window"])
	assert.Equal(t, "#/inbox", line["route"])
	assert.Equal(t, at.Format(time.RFC3339), line["client_time"])
	assert.InDelta(t, 3, line["count"], 0)
	assert.Equal(t, "f1", line["flowId"])
	assert.Contains(t, line["stack"], "at load")
}

func TestClientLogClampsTheLevel(t *testing.T) {
	cases := map[string]zerolog.Level{
		"fatal":   zerolog.ErrorLevel,
		"panic":   zerolog.ErrorLevel,
		"trace":   zerolog.DebugLevel,
		"warn":    zerolog.WarnLevel,
		"":        zerolog.InfoLevel,
		"verbose": zerolog.InfoLevel,
	}
	for in, want := range cases {
		assert.Equal(t, want, clientLogLevel(in), in)
	}
}

func TestClientLogDropsEntriesBelowTheRootLevel(t *testing.T) {
	var buf bytes.Buffer
	svc := newClientLogService(zerolog.New(&buf).Level(zerolog.WarnLevel))

	svc.Write(t.Context(), []ClientLogEntry{{Level: "info", Message: "noise"}, {Level: "warn", Message: "kept"}})

	lines := clientLogLines(t, &buf)
	require.Len(t, lines, 1)
	assert.Equal(t, "kept", lines[0]["message"])
	assert.Equal(t, "warn", svc.Level(t.Context()))
}

func TestClientLogBoundsUntrustedInput(t *testing.T) {
	var buf bytes.Buffer
	svc := newClientLogService(zerolog.New(&buf))

	fields := map[string]any{"nested": map[string]any{"blob": strings.Repeat("x", clientLogMaxFieldValue*2)}}
	for i := range clientLogMaxFields + 10 {
		fields[strings.Repeat("k", i%5+1)+string(rune('a'+i%26))+string(rune('a'+i/26))] = i
	}
	svc.Write(t.Context(), []ClientLogEntry{{
		Level:   "warn",
		Message: strings.Repeat("m", clientLogMaxMessage*2),
		Stack:   strings.Repeat("s", clientLogMaxStack*2),
		Fields:  fields,
	}})

	line := clientLogLines(t, &buf)[0]
	message, _ := line["message"].(string)
	stack, _ := line["stack"].(string)
	assert.LessOrEqual(t, len(message), clientLogMaxMessage+len("…"))
	assert.LessOrEqual(t, len(stack), clientLogMaxStack+len("…"))
	assert.LessOrEqual(t, len(line), clientLogMaxFields+4)
	if nested, ok := line["nested"]; ok {
		assert.IsType(t, "", nested)
	}
}

func TestClientLogCollapsesAFloodIntoOneCount(t *testing.T) {
	var buf bytes.Buffer
	svc := newClientLogService(zerolog.New(&buf))

	flood := make([]ClientLogEntry, clientLogBurst+50)
	for i := range flood {
		flood[i] = ClientLogEntry{Level: "error", Message: "render loop"}
	}
	svc.Write(t.Context(), flood)
	assert.Len(t, clientLogLines(t, &buf), clientLogBurst)

	buf.Reset()
	time.Sleep(100 * time.Millisecond)
	svc.Write(t.Context(), []ClientLogEntry{{Level: "error", Message: "after"}})

	lines := clientLogLines(t, &buf)
	require.Len(t, lines, 2)
	assert.Equal(t, "webview log entries suppressed by the rate limit", lines[0]["message"])
	assert.InDelta(t, 50, lines[0]["suppressed"], 0)
	assert.Equal(t, "after", lines[1]["message"])
}

func TestClientLogComponentIsAlwaysUnderWebview(t *testing.T) {
	assert.Equal(t, "webview", clientLogComponent(""))
	assert.Equal(t, "webview", clientLogComponent("  !!"))
	assert.Equal(t, "webview.terminal-windows", clientLogComponent("Terminal-Windows"))
	assert.Equal(t, "webview.a.b", clientLogComponent("a.b."))
}
