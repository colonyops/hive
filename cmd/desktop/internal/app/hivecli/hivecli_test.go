package hivecli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func appExecutable(t *testing.T, dir string) string {
	t.Helper()
	exe := filepath.Join(dir, "Hive.app", "Contents", "MacOS", executableName)
	require.NoError(t, os.MkdirAll(filepath.Dir(exe), 0o755))
	require.NoError(t, os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755))
	return exe
}

func TestSync(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T, link, exe, other string)
		enabled bool
		want    Action
		target  string // "exe", "other", "" for no link, "foreign" for the untouched file
	}{
		{name: "missing and wanted", enabled: true, want: ActionCreated, target: "exe"},
		{name: "missing and unwanted", enabled: false, want: ActionNone, target: ""},
		{
			name:    "already ours",
			setup:   func(t *testing.T, link, exe, _ string) { require.NoError(t, os.Symlink(exe, link)) },
			enabled: true, want: ActionNone, target: "exe",
		},
		{
			name:    "ours at a moved install",
			setup:   func(t *testing.T, link, _, other string) { require.NoError(t, os.Symlink(other, link)) },
			enabled: true, want: ActionRepointed, target: "exe",
		},
		{
			name:    "ours and unwanted",
			setup:   func(t *testing.T, link, exe, _ string) { require.NoError(t, os.Symlink(exe, link)) },
			enabled: false, want: ActionRemoved, target: "",
		},
		{
			name: "a foreign binary is wanted over",
			setup: func(t *testing.T, link, _, _ string) {
				require.NoError(t, os.WriteFile(link, []byte("brew"), 0o755))
			},
			enabled: true, want: ActionConflict, target: "foreign",
		},
		{
			name: "a foreign binary survives turning it off",
			setup: func(t *testing.T, link, _, _ string) {
				require.NoError(t, os.WriteFile(link, []byte("brew"), 0o755))
			},
			enabled: false, want: ActionNone, target: "foreign",
		},
		{
			name: "a foreign link is not ours",
			setup: func(t *testing.T, link, _, _ string) {
				require.NoError(t, os.Symlink("/opt/homebrew/Caskroom/hive/0.58.0/hive", link))
			},
			enabled: false, want: ActionNone, target: "foreign",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			exe := appExecutable(t, filepath.Join(root, "Applications"))
			other := appExecutable(t, filepath.Join(root, "Downloads"))
			link := LinkPath(filepath.Join(root, "home"))
			if tt.setup != nil {
				require.NoError(t, os.MkdirAll(filepath.Dir(link), 0o755))
				tt.setup(t, link, exe, other)
			}

			got, err := Sync(link, exe, tt.enabled)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)

			after, err := Inspect(link)
			require.NoError(t, err)
			switch tt.target {
			case "":
				assert.False(t, after.Exists)
			case "exe":
				assert.True(t, after.AppOwned)
				assert.Equal(t, exe, after.Target)
			case "foreign":
				assert.True(t, after.Exists)
				assert.False(t, after.AppOwned)
			}
		})
	}
}

func TestUnsupported(t *testing.T) {
	assert.NotEmpty(t, Unsupported("/Applications/Hive.app/Contents/MacOS/hive-desktop", "dev"))
	assert.NotEmpty(t, Unsupported("/private/var/folders/x/AppTranslocation/ABC/d/Hive.app/Contents/MacOS/hive-desktop", "0.61.0"))
	assert.NotEmpty(t, Unsupported("/tmp/hive-desktop-server", "0.61.0"))
	assert.Empty(t, Unsupported("/Applications/Hive.app/Contents/MacOS/hive-desktop", "0.61.0"))
}

func TestResolvePicksTheFirstExecutable(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(first, CommandName), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(second, CommandName), []byte("x"), 0o755))

	got := Resolve(strings.Join([]string{"", first, second}, string(os.PathListSeparator)))
	assert.Equal(t, filepath.Join(second, CommandName), got, "a file without an exec bit does not run")
	assert.Empty(t, Resolve(""))
}

func TestOnPath(t *testing.T) {
	assert.True(t, OnPath("/usr/bin:/home/u/.local/bin/", "/home/u/.local/bin"))
	assert.False(t, OnPath("/usr/bin", "/home/u/.local/bin"))
}

func TestParseVersion(t *testing.T) {
	assert.Equal(t, "v0.60.0", ParseVersion("hive version v0.60.0 (0908d97) 2026-10-05T03:20:51Z\n"))
	assert.Equal(t, "dev", ParseVersion("hive version dev (HEAD) now"))
	assert.Empty(t, ParseVersion("zsh: command not found: hive"))
}

func TestSameVersion(t *testing.T) {
	assert.True(t, SameVersion("v0.60.0", "0.60.0"))
	assert.False(t, SameVersion("0.60.0", "0.61.0"))
}
