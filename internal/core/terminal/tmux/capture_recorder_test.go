package tmux

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSONLCaptureRecorder_Record(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "recordings")
	recorder, err := NewJSONLCaptureRecorder(dir)
	require.NoError(t, err)

	observation := CaptureObservation{
		SessionName: "secret-session-name",
		PaneID:      "%42",
		Tool:        "claude",
		Content:     "agent output\n❯",
		Status:      terminal.StatusReady,
	}
	require.NoError(t, recorder.Record(observation))

	dirInfo, err := os.Stat(dir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), dirInfo.Mode().Perm())
	fileInfo, err := os.Stat(recorder.Path())
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), fileInfo.Mode().Perm())

	data, err := os.ReadFile(recorder.Path())
	require.NoError(t, err)
	assert.NotContains(t, string(data), observation.SessionName)
	assert.NotContains(t, string(data), observation.PaneID)

	var record captureRecord
	require.NoError(t, json.Unmarshal(data, &record))
	assert.Equal(t, captureRecordSchemaVersion, record.SchemaVersion)
	assert.NotEmpty(t, record.RunID)
	assert.NotEmpty(t, record.SessionKey)
	assert.NotEmpty(t, record.PaneKey)
	assert.NotEqual(t, record.SessionKey, record.PaneKey)
	assert.Equal(t, observation.Tool, record.Tool)
	assert.Equal(t, observation.Content, record.Content)
	assert.Len(t, record.ContentSHA256, 64)
	assert.False(t, record.ContentTruncated)
	assert.Equal(t, terminal.StatusReady, record.WeakLabel)
	assert.Equal(t, weakLabelSource, record.WeakLabelSource)
	assert.False(t, record.CapturedAt.IsZero())

	sameMachineRecorder, err := NewJSONLCaptureRecorder(dir)
	require.NoError(t, err)
	require.NoError(t, sameMachineRecorder.Record(observation))
	sameMachineRecords := readCaptureRecords(t, sameMachineRecorder.Path())
	require.Len(t, sameMachineRecords, 1)
	assert.Equal(t, record.SessionKey, sameMachineRecords[0].SessionKey, "session grouping must remain stable across recorder runs")
	assert.Equal(t, record.PaneKey, sameMachineRecords[0].PaneKey, "pane grouping must remain stable across recorder runs")

	otherRecorder, err := NewJSONLCaptureRecorder(filepath.Join(t.TempDir(), "other"))
	require.NoError(t, err)
	require.NoError(t, otherRecorder.Record(observation))
	otherRecords := readCaptureRecords(t, otherRecorder.Path())
	require.Len(t, otherRecords, 1)
	assert.NotEqual(t, record.SessionKey, otherRecords[0].SessionKey, "different installations must use different identities")
	assert.NotEqual(t, record.PaneKey, otherRecords[0].PaneKey, "different installations must use different identities")
}

func TestJSONLCaptureRecorder_BoundsLastContentBytes(t *testing.T) {
	recorder, err := NewJSONLCaptureRecorder(t.TempDir())
	require.NoError(t, err)
	content := strings.Repeat("a", maxRecordedContentBytes) + "🙂tail"

	require.NoError(t, recorder.Record(CaptureObservation{SessionName: "s", PaneID: "%1", Content: content}))
	records := readCaptureRecords(t, recorder.Path())
	require.Len(t, records, 1)
	assert.True(t, records[0].ContentTruncated)
	assert.LessOrEqual(t, len(records[0].Content), maxRecordedContentBytes)
	assert.True(t, strings.HasSuffix(records[0].Content, "🙂tail"))
	assert.True(t, strings.HasPrefix(records[0].Content, "a"))
}

func TestJSONLCaptureRecorder_DeduplicatesPerPane(t *testing.T) {
	recorder, err := NewJSONLCaptureRecorder(t.TempDir())
	require.NoError(t, err)
	observation := CaptureObservation{SessionName: "s", PaneID: "%1", Content: "same"}

	require.NoError(t, recorder.Record(observation))
	require.NoError(t, recorder.Record(observation))
	observation.PaneID = "%2"
	require.NoError(t, recorder.Record(observation))

	assert.Len(t, readCaptureRecords(t, recorder.Path()), 2)
}

func TestNewJSONLCaptureRecorder_RejectsSymlinkDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require elevated privileges")
	}
	parent := t.TempDir()
	target := t.TempDir()
	dir := filepath.Join(parent, "tmux")
	require.NoError(t, os.Symlink(target, dir))

	_, err := NewJSONLCaptureRecorder(dir)
	require.ErrorContains(t, err, "must not be a symlink")
}

func TestJSONLCaptureRecorder_ConcurrentWritesAreValidJSONLines(t *testing.T) {
	recorder, err := NewJSONLCaptureRecorder(t.TempDir())
	require.NoError(t, err)

	const count = 32
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			err := recorder.Record(CaptureObservation{
				SessionName: "session",
				PaneID:      "%" + string(rune('A'+index)),
				Content:     strings.Repeat("x", index+1),
			})
			assert.NoError(t, err)
		}(i)
	}
	wg.Wait()

	assert.Len(t, readCaptureRecords(t, recorder.Path()), count)
}

func readCaptureRecords(t *testing.T, path string) []captureRecord {
	t.Helper()
	file, err := os.Open(path)
	require.NoError(t, err)
	defer func() { require.NoError(t, file.Close()) }()

	var records []captureRecord
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var record captureRecord
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &record))
		records = append(records, record)
	}
	require.NoError(t, scanner.Err())
	return records
}
