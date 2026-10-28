package daemonclient

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/hive/daemon/daemonpath"
)

func TestSpawnEnvPinsTheDaemonToTheCallersDataDirAndConfig(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	got := spawnEnv([]string{"HIVE_DATA_DIR=/old", "HIVE_CONFIG=rel/config.yaml", "X=1", "HIVE_CONFIG_EXTRA=y"}, "/new")

	assert.Equal(t, []string{
		"HIVE_CONFIG=" + filepath.Join(cwd, "rel/config.yaml"),
		"X=1",
		"HIVE_CONFIG_EXTRA=y",
		"HIVE_DATA_DIR=/new",
	}, got, "the daemon runs in the data dir, so a relative config path must be made absolute, and the caller's data dir replaces an inherited one")
}

func TestSpawnEnvKeepsAnEmptyConfig(t *testing.T) {
	assert.Equal(t, []string{"HIVE_CONFIG=", "HIVE_DATA_DIR=/d"}, spawnEnv([]string{"HIVE_CONFIG="}, "/d"))
}

// fakeDaemon writes a binary that exits with code at once, as a daemon does
// when another one holds the lock. It writes marker first, when marker is set.
func fakeDaemon(t *testing.T, code int, marker string) string {
	t.Helper()
	script := "#!/bin/sh\n"
	if marker != "" {
		script += "echo > " + marker + "\n"
	}
	script += fmt.Sprintf("exit %d\n", code)
	path := filepath.Join(t.TempDir(), "hive")
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	return path
}

func shortDataDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "hives")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func serveHealthz(t *testing.T, socket string) error {
	if err := daemonpath.PrepareSocketDir(socket); err != nil {
		return err
	}
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	var protocols http.Protocols
	protocols.SetUnencryptedHTTP2(true)
	server := &http.Server{Handler: mux, Protocols: &protocols, ReadHeaderTimeout: time.Second}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	return nil
}

func TestSpawnGivesWayToADaemonAnotherClientStarted(t *testing.T) {
	dataDir := shortDataDir(t)
	marker := filepath.Join(t.TempDir(), "exited")
	binary := fakeDaemon(t, 1, marker)

	served := make(chan error, 1)
	go func() {
		// The rival daemon answers only after this one has given up, so
		// Spawn must keep polling past the exit.
		for {
			if _, err := os.Stat(marker); err == nil {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		served <- serveHealthz(t, daemonpath.SocketPath(dataDir))
	}()

	require.NoError(t, Spawn(t.Context(), binary, dataDir, nil))
	require.NoError(t, <-served)
}

func TestSpawnReportsTheExitWhenNoDaemonAnswers(t *testing.T) {
	original := spawnReadyTimeout
	t.Cleanup(func() { spawnReadyTimeout = original })
	// Long enough for the fake binary to start and exit on a loaded machine.
	spawnReadyTimeout = 3 * time.Second
	dataDir := shortDataDir(t)

	err := Spawn(t.Context(), fakeDaemon(t, 3, ""), dataDir, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "exit status 3")
	assert.Contains(t, err.Error(), daemonpath.OutLogPath(dataDir))
}
