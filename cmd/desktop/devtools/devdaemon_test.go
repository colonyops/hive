package main

import (
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"

	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
)

func TestDevDaemonFromEnvOnlyTakesADaemonInsideTheWorktree(t *testing.T) {
	worktree := t.TempDir()
	instance := filepath.Join(worktree, ".hive-desktop")
	cases := []struct {
		name    string
		binary  string
		dataDir string
		want    bool
	}{
		{"the instance's own", filepath.Join(instance, "bin", "hive"), filepath.Join(instance, "hive"), true},
		{"no daemon in launch.env", "", filepath.Join(instance, "hive"), false},
		{"the installed hive data dir", filepath.Join(instance, "bin", "hive"), t.TempDir(), false},
		{"a binary outside", "/usr/local/bin/hive", filepath.Join(instance, "hive"), false},
		{"the worktree itself", filepath.Join(instance, "bin", "hive"), worktree, false},
		{"a relative path", filepath.Join(instance, "bin", "hive"), ".hive-desktop/hive", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(settings.EnvDaemonHivePath, tc.binary)
			t.Setenv(settings.EnvHiveDataDir, tc.dataDir)
			_, ok := devDaemonFromEnv(zerolog.Nop(), worktree)
			assert.Equal(t, tc.want, ok)
		})
	}
}

func TestDevDaemonStopWithNothingRunningReturns(t *testing.T) {
	t.Parallel()
	devDaemon{dataDir: t.TempDir(), logger: zerolog.Nop()}.stop()
}
