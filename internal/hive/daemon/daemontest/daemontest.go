// Package daemontest serves a hive daemon over its own hive.db for a test, so
// anything a client reads or writes there can only have gone through the
// daemon's API.
package daemontest

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/hive/daemon"
	"github.com/colonyops/hive/internal/hive/hivetest"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/pkg/executil/executiltest"
)

// Socket runs a daemon until the test ends and answers its socket path.
func Socket(t *testing.T, mux sessionsvc.Multiplexer, configure ...func(*daemon.Options)) string {
	t.Helper()
	dataDir := t.TempDir()
	ctx, cancel := context.WithCancel(t.Context())
	ready := make(chan string, 1)
	done := make(chan error, 1)
	opts := daemon.Options{
		Config:  hivetest.LoadConfig(t, dataDir),
		Ports:   hive.Ports{Executor: &executiltest.Exec{}, Mux: mux, DataDir: dataDir, Logger: zerolog.Nop()},
		Version: "test",
		OnReady: func(path string) { ready <- path },
	}
	for _, apply := range configure {
		apply(&opts)
	}
	go func() {
		done <- daemon.Run(ctx, opts)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("daemon did not stop")
		}
	})
	select {
	case socket := <-ready:
		return socket
	case err := <-done:
		t.Fatalf("daemon exited before it listened: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("daemon did not start listening")
	}
	return ""
}
