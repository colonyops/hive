package daemonclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	hiveconfig "github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/hive/daemon"
)

const (
	spawnReadyTimeout = 5 * time.Second
	spawnPollInterval = 50 * time.Millisecond
)

// Spawn starts `<binary> daemon run` detached from the caller (its own
// session, stdio to <dataDir>/logs/hived.out) and waits up to 5s for
// /healthz. HIVE_DATA_DIR is set to dataDir and HIVE_CONFIG is made
// absolute, since the daemon outlives the caller's working directory. A
// daemon another client started first also counts as ready.
func Spawn(ctx context.Context, binary, dataDir string, env []string) error {
	dataDir, err := filepath.Abs(dataDir)
	if err != nil {
		return fmt.Errorf("resolve data directory: %w", err)
	}
	logPath := daemon.OutLogPath(dataDir)
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return fmt.Errorf("create daemon log directory: %w", err)
	}
	out, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open daemon output log: %w", err)
	}
	defer func() { _ = out.Close() }()

	// Not exec.CommandContext: the daemon must outlive ctx and the caller.
	cmd := exec.Command(binary, "daemon", "run") //nolint:noctx // detached on purpose
	cmd.Env = spawnEnv(env, dataDir)
	cmd.Dir = dataDir
	cmd.Stdout = out
	cmd.Stderr = out
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s daemon run: %w", binary, err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()

	client := New(Options{Dialer: UnixDialer(daemon.SocketPath(dataDir))})
	defer client.HTTPClient().CloseIdleConnections()
	deadline := time.NewTimer(spawnReadyTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(spawnPollInterval)
	defer ticker.Stop()
	var exitErr error
	for {
		if healthy(ctx, client.HTTPClient()) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			if exitErr != nil {
				return fmt.Errorf("the daemon exited (%w); see %s", exitErr, logPath)
			}
			return fmt.Errorf("the daemon did not answer within %s; see %s", spawnReadyTimeout, logPath)
		case err := <-exited:
			// Exiting is not yet a failure: a daemon another client started
			// holds the lock, and this one gives way to it.
			exitErr = err
			if exitErr == nil {
				exitErr = errors.New("exit status 0")
			}
		case <-ticker.C:
		}
	}
}

func healthy(ctx context.Context, client *http.Client) bool {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/healthz", nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func spawnEnv(env []string, dataDir string) []string {
	out := make([]string, 0, len(env)+1)
	for _, pair := range env {
		name, value, _ := strings.Cut(pair, "=")
		switch name {
		case hiveconfig.EnvDataDir:
			continue
		case hiveconfig.EnvConfig:
			if value != "" {
				if abs, err := filepath.Abs(value); err == nil {
					pair = name + "=" + abs
				}
			}
		}
		out = append(out, pair)
	}
	return append(out, hiveconfig.EnvDataDir+"="+dataDir)
}
