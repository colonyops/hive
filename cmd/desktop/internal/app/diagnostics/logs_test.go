package diagnostics

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMixedLogsAndStableReferences(t *testing.T) {
	path := filepath.Join(t.TempDir(), "desktop.log")
	content := "2026-10-06T16:17:17-08:00 ERR failure session=review-347\n" + `{"time":"2026-10-07T00:17:18Z","level":"info","message":"window created","window_id":"@3"}` + "\nunparsed output\npartial"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	source, entries := Read("desktop", path)
	require.Empty(t, source.Error)
	require.Len(t, entries, 3)
	require.Equal(t, "2026-10-07T00:17:17Z", entries[0].Time)
	require.Equal(t, "error", entries[0].Level)
	require.Equal(t, "window created", entries[1].Message)
	require.Equal(t, "unknown", entries[2].Level)
	require.NoError(t, os.WriteFile(path, []byte(content+" completed\n"), 0o600))
	_, again := Read("desktop", path)
	require.Len(t, again, 4)
	require.Equal(t, entries[0].ID, again[0].ID)
	require.Equal(t, "partial completed", again[3].Raw)
}

func TestReadBoundsAndMissingFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log")
	source, entries := Read("cli", path)
	require.NotEmpty(t, source.Error)
	require.Empty(t, entries)
	require.NoError(t, os.WriteFile(path, []byte(strings.Repeat("x", MaxTailBytes)+"\n2026-10-07T00:00:00Z INF kept\n"), 0o600))
	source, entries = Read("cli", path)
	require.True(t, source.Truncated)
	require.Len(t, entries, 1)
	require.Equal(t, "kept", entries[0].Message)
	entry := Parse("cli", "2026-10-07T00:00:00Z INF "+strings.Repeat("x", MaxEntryBytes*2))
	require.True(t, entry.Truncated)
	require.LessOrEqual(t, len(entry.Raw), MaxEntryBytes)
}

func TestCLIPathOverrides(t *testing.T) {
	require.Equal(t, "/explicit/log", CLIPath([]string{"HIVE_LOG_FILE=/explicit/log", "HIVE_DATA_DIR=/other"}, "/fallback"))
	require.Equal(t, "/data/hive.log", CLIPath([]string{"HIVE_DATA_DIR=/data", "XDG_DATA_HOME=/xdg"}, "/fallback"))
	require.Equal(t, "/xdg/hive/hive.log", CLIPath([]string{"XDG_DATA_HOME=/xdg"}, "/fallback"))
	require.Equal(t, "/fallback/hive.log", CLIPath(nil, "/fallback"))
}
