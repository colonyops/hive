package daemon_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/domain/terminal"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/hive/daemon"
	daemonv1 "github.com/colonyops/hive/internal/hive/daemon/gen/hive/daemon/v1"
	"github.com/colonyops/hive/internal/hive/daemon/gen/hive/daemon/v1/daemonv1connect"
	"github.com/colonyops/hive/internal/hive/hivetest"
	"github.com/colonyops/hive/pkg/executil/executiltest"
)

type running struct {
	socket string
	done   chan error
	cancel context.CancelFunc
}

type daemonOpts struct {
	waitLock     bool
	paneSource   terminal.PaneSource
	pollInterval time.Duration
}

// startDaemon reports a failure instead of failing the test, so a test can
// call it off the test goroutine.
func startDaemon(t *testing.T, dataDir string, opts daemonOpts) (running, error) {
	t.Helper()
	cfg := hivetest.LoadConfig(t, dataDir)
	if opts.pollInterval > 0 {
		cfg.Tmux.PollInterval = opts.pollInterval
	}
	ctx, cancel := context.WithCancel(t.Context())
	ready := make(chan string, 1)
	r := running{done: make(chan error, 1), cancel: cancel}
	go func() {
		r.done <- daemon.Run(ctx, daemon.Options{
			Config: cfg,
			Ports: hive.Ports{
				Executor:   &executiltest.Exec{},
				Mux:        hivetest.Mux{},
				PaneSource: opts.paneSource,
				DataDir:    dataDir,
				Logger:     zerolog.Nop(),
			},
			Version:  "test",
			WaitLock: opts.waitLock,
			OnReady:  func(path string) { ready <- path },
		})
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-r.done:
		case <-time.After(5 * time.Second):
			t.Error("daemon did not stop")
		}
	})
	select {
	case r.socket = <-ready:
		return r, nil
	case err := <-r.done:
		r.done <- err
		return r, fmt.Errorf("daemon exited before it listened: %w", err)
	case <-time.After(5 * time.Second):
		return r, errors.New("daemon did not start listening")
	}
}

func runDaemonWith(t *testing.T, dataDir string, opts daemonOpts) running {
	t.Helper()
	r, err := startDaemon(t, dataDir, opts)
	require.NoError(t, err)
	return r
}

func runDaemon(t *testing.T, dataDir string, waitLock bool) running {
	t.Helper()
	return runDaemonWith(t, dataDir, daemonOpts{waitLock: waitLock})
}

func httpClient(socket string) *http.Client {
	var protocols http.Protocols
	protocols.SetUnencryptedHTTP2(true)
	return &http.Client{Transport: &http.Transport{
		Protocols: &protocols,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socket)
		},
	}}
}

func TestRunServesHelloOnAPrivateSocket(t *testing.T) {
	dataDir := t.TempDir()
	d := runDaemon(t, dataDir, false)

	info, err := os.Stat(d.socket)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	client := daemonv1connect.NewDaemonServiceClient(httpClient(d.socket), "http://hived")
	hello, err := client.Hello(t.Context(), connect.NewRequest(&daemonv1.HelloRequest{}))
	require.NoError(t, err)
	assert.Equal(t, "test", hello.Msg.GetVersion())
	assert.Equal(t, int32(os.Getpid()), hello.Msg.GetPid()) //nolint:gosec // test pid
	assert.False(t, hello.Msg.GetStatusAvailable())

	resp, err := httpClient(d.socket).Get("http://hived/healthz")
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestRunRefusesASecondInstance(t *testing.T) {
	dataDir := t.TempDir()
	runDaemon(t, dataDir, false)

	err := daemon.Run(t.Context(), daemon.Options{
		Config: hivetest.LoadConfig(t, dataDir),
		Ports:  hive.Ports{Executor: &executiltest.Exec{}, Mux: hivetest.Mux{}, DataDir: dataDir, Logger: zerolog.Nop()},
	})
	require.ErrorIs(t, err, daemon.ErrAlreadyRunning)
}

func TestRunWithWaitLockStartsWhenTheFirstExits(t *testing.T) {
	dataDir := t.TempDir()
	first := runDaemon(t, dataDir, false)

	second := make(chan error, 1)
	go func() {
		_, err := startDaemon(t, dataDir, daemonOpts{waitLock: true})
		second <- err
	}()

	select {
	case <-second:
		t.Fatal("the waiting daemon started while the first held the lock")
	case <-time.After(200 * time.Millisecond):
	}

	stop := daemonv1connect.NewDaemonServiceClient(httpClient(first.socket), "http://hived")
	_, err := stop.Stop(t.Context(), connect.NewRequest(&daemonv1.StopRequest{}))
	require.NoError(t, err)
	select {
	case err := <-first.done:
		require.NoError(t, err)
		first.done <- nil
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not end the first daemon")
	}

	select {
	case err := <-second:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("the waiting daemon never started")
	}
}

func TestRunRemovesTheSocketOnExit(t *testing.T) {
	d := runDaemon(t, t.TempDir(), false)
	d.cancel()
	require.NoError(t, <-d.done)
	d.done <- nil
	_, err := os.Stat(d.socket)
	assert.ErrorIs(t, err, os.ErrNotExist)
}
