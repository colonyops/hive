//go:build integration

package integration

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// DriveServer is a running hive-drive-server with one signed-in account.
type DriveServer struct {
	URL   string
	Token string
}

const driveServerEmail = "test@example.com"

// StartDriveServer runs hive-drive-server on a free loopback port with temp
// data and config directories, and mints a push token for an account the
// server's policy admits.
//
// The admin commands validate the whole config, so the base URL has to be
// known before the server binds. The port is picked first and passed to
// --listen.
func (h *Harness) StartDriveServer(t *testing.T) DriveServer {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())

	dataDir, configDir := t.TempDir(), t.TempDir()
	policy := "user_policies:\n  - name: test\n    matches:\n      - \"*@example.com\"\n    quotas:\n      limit: 1GiB\n"
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "drive-server.yaml"), []byte(policy), 0o644))

	baseURL := "http://" + addr
	env := append(os.Environ(),
		"HIVE_DRIVE_SERVER_BASE_URL="+baseURL,
		"HIVE_DRIVE_SERVER_DATA_DIR="+dataDir,
		"HIVE_DRIVE_SERVER_CONFIG_DIR="+configDir,
	)

	ctx, cancel := context.WithCancel(context.Background())
	var logs bytes.Buffer
	serve := exec.CommandContext(ctx, "hive-drive-server", "serve", "--listen", addr)
	serve.Env = env
	serve.Stdout, serve.Stderr = &logs, &logs
	require.NoError(t, serve.Start())
	t.Cleanup(func() {
		cancel()
		_ = serve.Wait()
		if t.Failed() {
			t.Logf("hive-drive-server output:\n%s", logs.String())
		}
	})

	require.Eventually(t, func() bool {
		resp, err := http.Get(baseURL + "/healthz")
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 15*time.Second, 100*time.Millisecond, "drive server did not become healthy:\n%s", &logs)

	mint := exec.Command("hive-drive-server", "accounts", "token", "create", driveServerEmail, "--scope", "push")
	mint.Env = env
	var stderr bytes.Buffer
	mint.Stderr = &stderr
	out, err := mint.Output()
	require.NoError(t, err, "mint token: %s", stderr.String())
	token := strings.TrimSpace(string(out))
	require.NotEmpty(t, token, fmt.Sprintf("token create printed nothing: %s", stderr.String()))

	return DriveServer{URL: baseURL, Token: token}
}

// UseDriveServer points every later hive invocation at s through the
// environment. The container has no keychain, so there is no login step.
func (h *Harness) UseDriveServer(s DriveServer) *Harness {
	return h.WithEnv("HIVE_DRIVE_SERVER", s.URL).WithEnv("HIVE_DRIVE_TOKEN", s.Token)
}
