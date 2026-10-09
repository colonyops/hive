//go:build !windows

package tmuxexec

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLaunchLockHelper(t *testing.T) {
	if os.Getenv("HIVE_LAUNCH_LOCK_HELPER") != "1" {
		return
	}
	release, err := (execRunner{}).lockLaunch(t.Context(), "cross-process-launch-test")
	require.NoError(t, err)
	defer func() { require.NoError(t, release()) }()
	fmt.Println("locked")
	_, err = io.Copy(io.Discard, os.Stdin)
	require.NoError(t, err)
}

func TestLaunchLockAcrossProcesses(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("TMUX_TMPDIR", t.TempDir())
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestLaunchLockHelper$")
	cmd.Env = append(os.Environ(), "HIVE_LAUNCH_LOCK_HELPER=1")
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = stdin.Close(); _ = cmd.Process.Kill(); _ = cmd.Wait() })
	line, err := bufio.NewReader(stdout).ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "locked\n", line)
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Millisecond)
	defer cancel()
	_, err = (execRunner{}).lockLaunch(ctx, "cross-process-launch-test")
	require.ErrorIs(t, err, context.DeadlineExceeded)
	release, err := (execRunner{}).lockLaunch(t.Context(), "another-session")
	require.NoError(t, err)
	require.NoError(t, release())
	require.NoError(t, stdin.Close())
	require.NoError(t, cmd.Wait())
	release, err = (execRunner{}).lockLaunch(t.Context(), "cross-process-launch-test")
	require.NoError(t, err)
	require.NoError(t, release())
}

func TestLaunchLockFilesAreBoundedPerServer(t *testing.T) {
	names := make(map[string]struct{})
	for i := range 1000 {
		names[launchLockName("/tmp/tmux-501/default", fmt.Sprintf("session-%d", i))] = struct{}{}
	}
	require.LessOrEqual(t, len(names), launchLockBuckets)
	require.NotEqual(t, launchLockName("/tmp/tmux-501/default", "session"), launchLockName("/tmp/tmux-501/other", "session"))
}

func TestLaunchLockDistinguishesServers(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	runner := func(socket string) execRunner {
		return execRunner{prepareArgs: func(args []string) []string { return append([]string{"-L", socket}, args...) }}
	}
	release, err := runner("one").lockLaunch(t.Context(), "session")
	require.NoError(t, err)
	defer func() { require.NoError(t, release()) }()
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Millisecond)
	defer cancel()
	releaseOther, err := runner("two").lockLaunch(ctx, "session")
	require.NoError(t, err)
	require.NoError(t, releaseOther())
}
