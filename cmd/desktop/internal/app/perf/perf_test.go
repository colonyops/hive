package perf

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestRecorder(t *testing.T, maxBytes int64) (*Recorder, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), FileName)
	r, err := New(Options{Path: path, MaxBytes: maxBytes})
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })
	return r, path
}

func readLines(t *testing.T, path string) []record {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)

	var out []record
	for line := range strings.SplitSeq(strings.TrimSpace(string(raw)), "\n") {
		if line == "" {
			continue
		}
		var rec record
		require.NoError(t, json.Unmarshal([]byte(line), &rec))
		out = append(out, rec)
	}
	return out
}

func TestRecordWritesOneLinePerSampleInWriteOrder(t *testing.T) {
	r, path := newTestRecorder(t, DefaultMaxBytes)

	n, err := r.Record(
		Sample{Scope: "feed", Name: "item:open", DurationMs: 12.5},
		Sample{Scope: "feed", Name: "item:close", DurationMs: 3},
	)
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	lines := readLines(t, path)
	require.Len(t, lines, 2)
	assert.Equal(t, uint64(1), lines[0].Seq)
	assert.Equal(t, "item:open", lines[0].Name)
	assert.InDelta(t, 12.5, lines[0].DurationMs, 0.001)
	assert.Equal(t, uint64(2), lines[1].Seq)
	assert.Equal(t, "item:close", lines[1].Name)
}

func TestRecordStampsMissingTimestamp(t *testing.T) {
	r, path := newTestRecorder(t, DefaultMaxBytes)

	explicit := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	_, err := r.Record(
		Sample{Scope: "feed", Name: "stamped", DurationMs: 1},
		Sample{Scope: "feed", Name: "explicit", DurationMs: 1, At: explicit},
	)
	require.NoError(t, err)

	lines := readLines(t, path)
	require.Len(t, lines, 2)
	assert.False(t, lines[0].At.IsZero(), "a sample with no timestamp is stamped at write time")
	assert.True(t, explicit.Equal(lines[1].At), "an explicit timestamp is preserved")
}

func TestRecordDropsInvalidSamplesWithoutFailingTheBatch(t *testing.T) {
	r, path := newTestRecorder(t, DefaultMaxBytes)

	n, err := r.Record(
		Sample{Scope: "feed", Name: "good", DurationMs: 1},
		Sample{Scope: "", Name: "no-scope", DurationMs: 1},
		Sample{Scope: "feed", Name: "", DurationMs: 1},
		Sample{Scope: "feed", Name: "negative", DurationMs: -1},
		Sample{Scope: "feed", Name: "also-good", DurationMs: 2},
	)
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	lines := readLines(t, path)
	require.Len(t, lines, 2)
	assert.Equal(t, "good", lines[0].Name)
	assert.Equal(t, "also-good", lines[1].Name)
	assert.Equal(t, uint64(2), lines[1].Seq, "dropped samples do not consume a sequence number")
}

func TestValidateBoundsAttributes(t *testing.T) {
	tests := []struct {
		name   string
		sample Sample
	}{
		{"too many attributes", Sample{Scope: "s", Name: "n", Attrs: func() map[string]any {
			attrs := make(map[string]any, maxAttrs+1)
			for i := range maxAttrs + 1 {
				attrs[string(rune('a'+i%26))+strings.Repeat("x", i)] = 1
			}
			return attrs
		}()}},
		{"oversized key", Sample{Scope: "s", Name: "n", Attrs: map[string]any{strings.Repeat("k", maxAttrLen+1): 1}}},
		{"oversized string value", Sample{Scope: "s", Name: "n", Attrs: map[string]any{"k": strings.Repeat("v", maxAttrLen+1)}}},
		{"oversized scope", Sample{Scope: strings.Repeat("s", maxScopeLen+1), Name: "n"}},
		{"oversized name", Sample{Scope: "s", Name: strings.Repeat("n", maxNameLen+1)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Error(t, tt.sample.Validate())
		})
	}
}

func TestDisabledRecorderWritesNothing(t *testing.T) {
	r := Off()

	assert.False(t, r.Enabled())
	assert.Empty(t, r.Path())

	n, err := r.Record(Sample{Scope: "feed", Name: "item:open", DurationMs: 1})
	assert.Equal(t, 0, n)
	require.ErrorIs(t, err, ErrDisabled)
	require.NoError(t, r.Close())
}

func TestRotationKeepsOnePreviousGeneration(t *testing.T) {
	r, path := newTestRecorder(t, 256)

	// Enough samples that the cap is crossed more than once, so the assertion
	// covers a second rotation overwriting the first .1 rather than accruing.
	for i := range 40 {
		_, err := r.Record(Sample{Scope: "feed", Name: "item:open", DurationMs: float64(i)})
		require.NoError(t, err)
	}

	current, err := os.Stat(path)
	require.NoError(t, err)
	assert.LessOrEqual(t, current.Size(), int64(256), "the live file stays within the cap")

	previous, err := os.Stat(path + ".1")
	require.NoError(t, err, "the previous generation is kept")
	assert.LessOrEqual(t, previous.Size(), int64(256))

	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	assert.Len(t, entries, 2, "rotation never accrues a third generation")

	lines := readLines(t, path)
	require.NotEmpty(t, lines)
	assert.Greater(t, lines[0].Seq, uint64(1), "the sequence continues across a rotation")
}

func TestNewAppendsToAnExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)

	first, err := New(Options{Path: path})
	require.NoError(t, err)
	_, err = first.Record(Sample{Scope: "feed", Name: "before", DurationMs: 1})
	require.NoError(t, err)
	require.NoError(t, first.Close())

	second, err := New(Options{Path: path})
	require.NoError(t, err)
	t.Cleanup(func() { _ = second.Close() })
	_, err = second.Record(Sample{Scope: "feed", Name: "after", DurationMs: 1})
	require.NoError(t, err)

	lines := readLines(t, path)
	require.Len(t, lines, 2)
	assert.Equal(t, "before", lines[0].Name)
	assert.Equal(t, "after", lines[1].Name)
}

func TestNewDefaultsTheCap(t *testing.T) {
	r, _ := newTestRecorder(t, 0)
	assert.Equal(t, int64(DefaultMaxBytes), r.MaxBytes())
}
