//go:build remote

// Package remote tests hive against real servers: two containers that run
// hive behind sshd. A test drives the client hive on this machine, the hive
// CLI on a server over ssh, or the session API over ssh, and checks the
// result from the server's own CLI. Run it with test/remote/run.sh.
package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive/daemonclient"
	"github.com/colonyops/hive/internal/platform/sshdial"
)

const (
	// tFixtureRepo is a bare repository with one commit on both servers.
	tFixtureRepo    = "/home/hive/repo.git"
	tCommandTimeout = 60 * time.Second
)

var (
	serverA = Server{Name: "server-a"}
	serverB = Server{Name: "server-b"}
)

type Result struct {
	Args   []string
	Stdout string
	Stderr string
	Err    error
}

func (r Result) Ok(t *testing.T) Result {
	t.Helper()
	require.NoError(t, r.Err, "%s\nstdout:\n%s\nstderr:\n%s", strings.Join(r.Args, " "), r.Stdout, r.Stderr)
	return r
}

func (r Result) Fails(t *testing.T) Result {
	t.Helper()
	require.Error(t, r.Err, "%s should fail\nstdout:\n%s", strings.Join(r.Args, " "), r.Stdout)
	return r
}

func run(t *testing.T, env []string, name string, args ...string) Result {
	t.Helper()
	// Not t.Context(): cleanups run commands too, after it is canceled.
	ctx, cancel := context.WithTimeout(context.Background(), tCommandTimeout)
	defer cancel()
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	if env != nil {
		cmd.Env = append(os.Environ(), env...)
	}
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	return Result{Args: append([]string{name}, args...), Stdout: stdout.String(), Stderr: stderr.String(), Err: err}
}

func DecodeJSON[T any](t *testing.T, r Result) T {
	t.Helper()
	var v T
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &v), "decode %q", r.Stdout)
	return v
}

func DecodeJSONLines[T any](t *testing.T, r Result) []T {
	t.Helper()
	var out []T
	dec := json.NewDecoder(strings.NewReader(r.Stdout))
	for {
		var v T
		err := dec.Decode(&v)
		if errors.Is(err, io.EOF) {
			return out
		}
		require.NoError(t, err, "decode %q", r.Stdout)
		out = append(out, v)
	}
}

// Session is the JSON that `hive session show --json` and its siblings
// print.
type Session struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Remote string   `json:"remote"`
	Path   string   `json:"path"`
	State  string   `json:"state"`
	Group  string   `json:"group"`
	Tags   []string `json:"tags"`
}

// Client is the hive CLI on this machine, with its own data dir and a
// config that lists both servers.
type Client struct {
	t       *testing.T
	dataDir string
	config  string
}

func NewClient(t *testing.T) *Client {
	t.Helper()
	dir := t.TempDir()
	config := filepath.Join(dir, "config.yaml")
	var servers strings.Builder
	servers.WriteString("servers:\n")
	for _, s := range []Server{serverA, serverB} {
		fmt.Fprintf(&servers, "  - name: %s\n    ssh: %s\n    ssh_config: %s\n", s.Name, s.Name, sshConfig)
	}
	require.NoError(t, os.WriteFile(config, []byte(servers.String()), 0o600))
	return &Client{t: t, dataDir: filepath.Join(dir, "data"), config: config}
}

func (c *Client) Hive(args ...string) Result {
	c.t.Helper()
	r := run(c.t, []string{"HIVE_CONFIG=" + c.config, "HIVE_DATA_DIR=" + c.dataDir}, hiveBin, args...)
	r.Args[0] = "hive"
	return r
}

// Server is one fixture container, reached by its name in the ssh_config
// run.sh writes.
type Server struct {
	Name string
}

// Hive runs the hive CLI on the server. Its config sets experimental.daemon,
// so the command goes through the same daemon a remote client reaches.
func (s Server) Hive(t *testing.T, args ...string) Result {
	t.Helper()
	remote := []string{"hive"}
	for _, arg := range args {
		remote = append(remote, shellQuote(arg))
	}
	sshArgs := []string{"-F", sshConfig, "-T", "-o", "BatchMode=yes", s.Name, "--", strings.Join(remote, " ")}
	r := run(t, nil, "ssh", sshArgs...)
	r.Args = append([]string{"ssh", s.Name, "hive"}, args...)
	return r
}

// API is the session API on the server over `ssh -- hive daemon dial-stdio`,
// the same path a remote client takes.
func (s Server) API(t *testing.T) *daemonclient.Client {
	t.Helper()
	host := sshdial.Host{Name: s.Name, ConfigFile: sshConfig}
	client := daemonclient.New(daemonclient.Options{
		Logger: zerolog.Nop(),
		Dialer: func(ctx context.Context) (net.Conn, error) {
			return sshdial.DialStdio(ctx, host, os.Stderr)
		},
	})
	t.Cleanup(client.HTTPClient().CloseIdleConnections)
	return client
}

// CreateSession creates a session on the server with its own CLI and
// deletes it when the test ends.
func (s Server) CreateSession(t *testing.T, name string) Session {
	t.Helper()
	r := s.Hive(t, "session", "create", "--json", "--background", "--remote", tFixtureRepo, name).Ok(t)
	sess := DecodeJSON[Session](t, r)
	s.DeleteOnCleanup(t, sess.ID)
	return sess
}

// DeleteOnCleanup deletes a session when the test ends, so the next test
// starts from the same server state. A session the test already deleted is
// fine.
func (s Server) DeleteOnCleanup(t *testing.T, id string) {
	t.Helper()
	t.Cleanup(func() {
		r := s.Hive(t, "session", "delete", "--force", id)
		// hive prints its errors to stdout.
		if r.Err != nil && !strings.Contains(r.Stdout+r.Stderr, session.ErrNotFound.Error()) {
			t.Logf("delete session %s on %s: %v\n%s%s", id, s.Name, r.Err, r.Stdout, r.Stderr)
		}
	})
}

func (s Server) Show(t *testing.T, id string) Session {
	t.Helper()
	return DecodeJSON[Session](t, s.Hive(t, "session", "show", "--json", id).Ok(t))
}

func (s Server) List(t *testing.T) []Session {
	t.Helper()
	return DecodeJSONLines[Session](t, s.Hive(t, "session", "list", "--json").Ok(t))
}

// Stop stops the server's container and starts it again when the test ends,
// so later tests find both servers up.
func (s Server) Stop(t *testing.T) {
	t.Helper()
	compose(t, "stop", s.Name).Ok(t)
	t.Cleanup(func() { s.Start(t) })
}

// Start starts a stopped server and waits for sshd. Its tmux sessions and
// daemon are gone; its hive.db is kept.
func (s Server) Start(t *testing.T) {
	t.Helper()
	compose(t, "up", "--detach", "--wait", "--no-build", s.Name).Ok(t)
}

func compose(t *testing.T, args ...string) Result {
	t.Helper()
	return run(t, nil, "docker", append([]string{"compose", "-p", project, "-f", "compose.yaml"}, args...)...)
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
