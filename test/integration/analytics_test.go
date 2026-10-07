//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	store "github.com/colonyops/hive/internal/store/usageanalytics"
	"github.com/stretchr/testify/require"
)

func TestUsageCommandsFlushBeforeExitAndShareSessionRecorder(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "usage-repo")
	_, err := h.RunStdout("ls")
	require.NoError(t, err)
	_, err = h.RunStdout("new", "--remote", repo, "usage-session")
	require.NoError(t, err)
	_, err = h.RunStdout("session", "show", "missing")
	require.Error(t, err)
	counts, err := store.ReadSummary(t.Context(), h.DataDir(), time.Now())
	require.NoError(t, err)
	require.EqualValues(t, 3, counts.CLICommands)
	require.EqualValues(t, 1, counts.HiveSessions)
}

func TestUsageDisabledHelpAndCompletionCreateNoHistory(t *testing.T) {
	for _, args := range [][]string{{"ls"}, {"--help"}, {"completion", "bash"}, {"--generate-shell-completion"}} {
		h := NewHarness(t)
		cmd := h.command(args...)
		if args[0] == "ls" {
			cmd.Env = append(cmd.Env, "HIVE_ANALYTICS_ENABLED=false")
		}
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, string(output))
		_, err = os.Stat(filepath.Join(h.DataDir(), store.Filename))
		require.True(t, os.IsNotExist(err), "%v", args)
	}
}
