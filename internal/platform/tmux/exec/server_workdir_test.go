//go:build !server

package tmuxexec_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tmuxexec "github.com/colonyops/hive/internal/platform/tmux/exec"
	"github.com/colonyops/hive/internal/platform/tmuxtest"
	"github.com/stretchr/testify/require"
)

// A server that starts while hive runs inside a session checkout must still
// spawn panes where -c says after that checkout is deleted.
func TestServerOutlivesTheCheckoutItStartedFrom(t *testing.T) {
	socket := tmuxtest.Private(t)
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_PANE", "")
	runner := tmuxexec.NewExecRunner(tmuxexec.ExecRunnerOptions{
		PrepareArgs: func(args []string) []string {
			return append([]string{"-S", socket, "-f", "/dev/null"}, args...)
		},
	})

	checkout := t.TempDir()
	t.Chdir(checkout)
	_, _, err := runner.Capture(t.Context(), "new-session", "-d", "-s", "first", "--", "sleep", "300")
	require.NoError(t, err)
	t.Chdir(t.TempDir())
	require.NoError(t, os.Remove(checkout))

	paneDir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	marker := filepath.Join(paneDir, "pwd")
	_, _, err = runner.Capture(t.Context(), "new-session", "-d", "-s", "probe", "-c", paneDir,
		"--", "sh", "-c", "/bin/pwd -P > '"+marker+"' 2>&1; sleep 300")
	require.NoError(t, err)

	var got string
	require.Eventually(t, func() bool {
		out, err := os.ReadFile(marker)
		got = strings.TrimSpace(string(out))
		return err == nil && got != ""
	}, 5*time.Second, 20*time.Millisecond)
	require.Equal(t, paneDir, got)
}
