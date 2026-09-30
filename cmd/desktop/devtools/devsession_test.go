package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestParseProcessTable(t *testing.T) {
	t.Parallel()

	table := parseProcessTable(`
  101     1 /opt/homebrew/bin/mise run dev
  102   101 wails3 dev -config ./build/config.yml
  bad   row skipped
  103   102 bin/Hive.dev.app/Contents/MacOS/hive-desktop
`)

	require.Equal(t, []procEntry{
		{pid: 101, ppid: 1, command: "/opt/homebrew/bin/mise run dev"},
		{pid: 102, ppid: 101, command: "wails3 dev -config ./build/config.yml"},
		{pid: 103, ppid: 102, command: "bin/Hive.dev.app/Contents/MacOS/hive-desktop"},
	}, table)
}

func TestDescendantsWalksTheWholeTree(t *testing.T) {
	t.Parallel()

	table := []procEntry{
		{pid: 1, ppid: 0, command: "init"},
		{pid: 10, ppid: 1, command: "mise"},
		{pid: 11, ppid: 10, command: "wails3 dev"},
		{pid: 12, ppid: 11, command: "wails3 task run"},
		{pid: 13, ppid: 12, command: "hive-desktop"},
		{pid: 14, ppid: 11, command: "wails3 task common:dev:frontend"},
		{pid: 20, ppid: 1, command: "someone else's wails3 dev"},
	}

	var pids []int
	for _, entry := range descendants(table, 11) {
		pids = append(pids, entry.pid)
	}
	require.ElementsMatch(t, []int{12, 13, 14}, pids, "another worktree's tree must never be swept in")
}

func TestCommandName(t *testing.T) {
	t.Parallel()

	require.Equal(t, appProcessName, commandName("bin/Hive.dev.app/Contents/MacOS/hive-desktop"))
	require.Equal(t, appProcessName, commandName("/repo/desktop/bin/hive-desktop --flag"))
	// The rebuild that produces the binary must not be mistaken for it.
	require.Equal(t, "go", commandName("go build -o ./bin/hive-desktop ./desktop"))
}

func TestWatchProcessFiresWhenTheProcessIsGone(t *testing.T) {
	t.Parallel()

	// Started and reaped, so the pid is known dead rather than merely unlikely.
	sleeper := exec.Command("/bin/sh", "-c", "exit 0")
	require.NoError(t, sleeper.Start())
	pid := sleeper.Process.Pid
	require.NoError(t, sleeper.Wait())

	gone := make(chan struct{})
	go watchProcess(pid, time.Millisecond, gone)

	select {
	case <-gone:
	case <-time.After(5 * time.Second):
		t.Fatal("watchProcess never noticed the process was gone")
	}
}

func TestWatchProcessStaysQuietWhileTheProcessLives(t *testing.T) {
	t.Parallel()

	gone := make(chan struct{})
	go watchProcess(os.Getpid(), time.Millisecond, gone)

	select {
	case <-gone:
		t.Fatal("watchProcess fired on a live process; a dev session would tear itself down")
	case <-time.After(200 * time.Millisecond):
	}
}

// TestRunDevSessionStopsTheAppAndLeavesNothing is the regression for the leak
// this supervisor exists to close: a terminal that closes signals only the
// foreground group, and the dev runner then dies before killing the app it put
// in a group of its own. The fake runner deliberately does not forward anything
// to its child, so the app is stopped here or not at all.
func TestRunDevSessionStopsTheAppAndLeavesNothing(t *testing.T) {
	dir := t.TempDir()

	// The app has to be a real executable whose argv[0] is named like the dev
	// binary, since that base name is how the supervisor picks it out.
	fakeApp := filepath.Join(dir, appProcessName)
	require.NoError(t, os.Symlink("/bin/sh", fakeApp))

	appStarted := filepath.Join(dir, "app-started")
	appStopped := filepath.Join(dir, "app-stopped")
	appPID := filepath.Join(dir, "app-pid")
	runnerStopped := filepath.Join(dir, "runner-stopped")

	appScript := filepath.Join(dir, "app.sh")
	require.NoError(t, os.WriteFile(appScript, []byte(
		"trap 'printf stopped > "+appStopped+"; exit 0' TERM\n"+
			"printf $$ > "+appPID+"\n"+
			"printf started > "+appStarted+"\n"+
			"while :; do sleep 0.05; done\n"), 0o600))

	// It outlives its child on purpose: the real runner keeps supervising after
	// the app exits, so nothing here ends unless the supervisor ends it.
	runnerScript := filepath.Join(dir, "runner.sh")
	require.NoError(t, os.WriteFile(runnerScript, []byte(
		"trap 'printf stopped > "+runnerStopped+"; exit 0' INT\n"+
			fakeApp+" "+appScript+" &\n"+
			"while :; do sleep 0.05; done\n"), 0o600))

	done := make(chan error, 1)
	go func() { done <- runDevSession(zerolog.Nop(), []string{"/bin/sh", runnerScript}) }()

	require.Eventually(t, func() bool {
		_, err := os.Stat(appStarted)
		return err == nil
	}, 10*time.Second, 20*time.Millisecond, "fake app never started")

	// What a closing terminal delivers, and what neither the dev runner nor its
	// watcher library handles.
	require.NoError(t, syscall.Kill(os.Getpid(), syscall.SIGHUP))

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(30 * time.Second):
		t.Fatal("the supervisor never returned")
	}

	require.FileExists(t, appStopped, "the app was never asked to shut down")
	require.FileExists(t, runnerStopped, "the dev runner was never interrupted")

	raw, err := os.ReadFile(appPID)
	require.NoError(t, err)
	pid := 0
	_, err = fmt.Sscanf(string(raw), "%d", &pid)
	require.NoError(t, err)
	require.Error(t, syscall.Kill(pid, syscall.Signal(0)), "the app outlived the dev session")
}
