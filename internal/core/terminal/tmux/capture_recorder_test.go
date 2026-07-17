package tmux

import (
	"crypto/sha256"
	"encoding/hex"
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

func TestJSONCaptureRecorder_Record(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "recordings")
	recorder, err := NewJSONCaptureRecorder(dir)
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

	path := capturePath(dir, observation.Content)
	fileInfo, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), fileInfo.Mode().Perm())
	assert.Equal(t, filepath.Base(path), sha256Hex(observation.Content)+".json")

	data, err := os.ReadFile(path)
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
	assert.Equal(t, sha256Hex(observation.Content), record.ContentSHA256)
	assert.Equal(t, terminal.StatusReady, record.WeakLabel)
	assert.Equal(t, weakLabelSource, record.WeakLabelSource)
	assert.False(t, record.CapturedAt.IsZero())

	sameMachineRecorder, err := NewJSONCaptureRecorder(dir)
	require.NoError(t, err)
	assert.Equal(t, record.SessionKey, sameMachineRecorder.opaqueKey("session", observation.SessionName))
	assert.Equal(t, record.PaneKey, sameMachineRecorder.opaqueKey("pane", observation.SessionName, observation.PaneID))

	otherRecorder, err := NewJSONCaptureRecorder(filepath.Join(t.TempDir(), "other"))
	require.NoError(t, err)
	assert.NotEqual(t, record.SessionKey, otherRecorder.opaqueKey("session", observation.SessionName))
	assert.NotEqual(t, record.PaneKey, otherRecorder.opaqueKey("pane", observation.SessionName, observation.PaneID))
}

func TestJSONCaptureRecorder_StoresWholeCapture(t *testing.T) {
	recorder, err := NewJSONCaptureRecorder(t.TempDir())
	require.NoError(t, err)
	content := strings.Repeat("0123456789🙂", 4096)

	require.NoError(t, recorder.Record(CaptureObservation{SessionName: "s", PaneID: "%1", Content: content}))
	record := readCaptureRecord(t, capturePath(recorder.Dir(), content))
	assert.Equal(t, content, record.Content)
	assert.Greater(t, len(record.Content), 8*1024)
}

func TestJSONCaptureRecorder_DeduplicatesByContentAcrossRecorders(t *testing.T) {
	dir := t.TempDir()
	first, err := NewJSONCaptureRecorder(dir)
	require.NoError(t, err)
	second, err := NewJSONCaptureRecorder(dir)
	require.NoError(t, err)

	observation := CaptureObservation{
		SessionName: "s",
		PaneID:      "%1",
		Tool:        "claude",
		Content:     "same",
		Status:      terminal.StatusReady,
	}
	require.NoError(t, first.Record(observation))
	observation.SessionName = "another-session"
	observation.PaneID = "%9"
	observation.Tool = "codex"
	observation.Status = terminal.StatusActive
	require.NoError(t, second.Record(observation))

	assert.Len(t, captureFiles(t, dir), 1)
	record := readCaptureRecord(t, capturePath(dir, observation.Content))
	assert.Equal(t, "claude", record.Tool, "the first observation owns metadata for deduplicated content")
	assert.Equal(t, terminal.StatusReady, record.WeakLabel)
}

func TestNewJSONCaptureRecorder_RejectsSymlinkDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require elevated privileges")
	}
	parent := t.TempDir()
	target := t.TempDir()
	dir := filepath.Join(parent, "tmux")
	require.NoError(t, os.Symlink(target, dir))

	_, err := NewJSONCaptureRecorder(dir)
	require.ErrorContains(t, err, "must not be a symlink")
}

func TestJSONCaptureRecorder_ConcurrentDeduplication(t *testing.T) {
	dir := t.TempDir()
	recorder, err := NewJSONCaptureRecorder(dir)
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
				Content:     "identical-content",
			})
			assert.NoError(t, err)
		}(i)
	}
	wg.Wait()

	assert.Len(t, captureFiles(t, dir), 1)
	_ = readCaptureRecord(t, capturePath(dir, "identical-content"))
}

func capturePath(dir, content string) string {
	return filepath.Join(dir, sha256Hex(content)+".json")
}

func sha256Hex(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])
}

func captureFiles(t *testing.T, dir string) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	require.NoError(t, err)
	return paths
}

func readCaptureRecord(t *testing.T, path string) captureRecord {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var record captureRecord
	require.NoError(t, json.Unmarshal(data, &record))
	return record
}
