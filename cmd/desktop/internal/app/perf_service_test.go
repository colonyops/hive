package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/perf"
	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
)

func TestPerfRecordingReachesTheFileWhenTheSettingIsOn(t *testing.T) {
	root := t.TempDir()
	t.Setenv(settings.EnvDataDir, filepath.Join(root, "data"))
	t.Setenv("HIVE_CONFIG", filepath.Join(root, "hive.yaml"))
	t.Setenv(settings.EnvConfigDir, filepath.Join(root, "config"))
	t.Setenv(settings.EnvMockMode, "feed")

	cfg := settings.DefaultSettings()
	cfg.Development.Perf.Enabled = true

	core, err := New(t.Context(), Config{
		Settings: cfg,
		MockMode: settings.MockMode(),
		Logger:   zerolog.Nop(),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = core.Close() })

	info := core.Perf.Info(t.Context())
	require.True(t, info.Enabled)
	assert.Equal(t, filepath.Join(root, "data", "desktop", perf.FileName), info.Path)

	written, err := core.Perf.Record(t.Context(), []PerfSample{
		{Scope: "feed", Name: "item:open", DurationMs: 12.5, Attrs: map[string]any{"itemId": "i1"}},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, written)

	require.NoError(t, core.Close())

	raw, err := os.ReadFile(info.Path)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"name":"item:open"`)
	assert.Contains(t, string(raw), `"itemId":"i1"`)
}

func TestPerfRecordingIsInertWhenTheSettingIsOff(t *testing.T) {
	root := t.TempDir()
	t.Setenv(settings.EnvDataDir, filepath.Join(root, "data"))
	t.Setenv("HIVE_CONFIG", filepath.Join(root, "hive.yaml"))
	t.Setenv(settings.EnvConfigDir, filepath.Join(root, "config"))
	t.Setenv(settings.EnvMockMode, "feed")

	core, err := New(t.Context(), Config{
		Settings: settings.DefaultSettings(),
		MockMode: settings.MockMode(),
		Logger:   zerolog.Nop(),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = core.Close() })

	assert.False(t, core.Perf.Info(t.Context()).Enabled)

	// Not an error: instrumentation left in the code is expected to run
	// against a build with recording off.
	written, err := core.Perf.Record(t.Context(), []PerfSample{{Scope: "feed", Name: "item:open", DurationMs: 1}})
	require.NoError(t, err)
	assert.Zero(t, written)

	_, err = os.Stat(filepath.Join(root, "data", "desktop", perf.FileName))
	assert.ErrorIs(t, err, os.ErrNotExist, "a disabled recorder never opens the file")
}

func TestOpenPerfRecorderDegradesWhenTheFileCannotBeOpened(t *testing.T) {
	// A regular file where the state directory should be: MkdirAll fails, and
	// startup has to continue without recording rather than refuse to boot.
	blocked := filepath.Join(t.TempDir(), "blocked")
	require.NoError(t, os.WriteFile(blocked, []byte("not a directory"), 0o600))

	var logs bytes.Buffer
	recorder := openPerfRecorder(true, filepath.Join(blocked, "state"), zerolog.New(&logs))

	assert.False(t, recorder.Enabled())
	assert.Contains(t, logs.String(), "perf recording disabled")
}
