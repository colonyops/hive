package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
	"github.com/colonyops/hive/internal/hive/daemonclient"
)

const (
	daemonStopTimeout = 10 * time.Second
	daemonStopPoll    = 100 * time.Millisecond
)

// devDaemon is the hive daemon of a development instance: this worktree's hive,
// built into the instance, serving the instance's own hive data dir. A daemon
// is the one writer of its data dir, so a branch build cannot share the
// installed hive's dir with the installed daemon.
type devDaemon struct {
	worktree string
	binary   string
	dataDir  string
	logger   zerolog.Logger
}

// devDaemonFromEnv reads the daemon that launch.env names. It answers false for
// a launch.env without one, and for a data dir outside the worktree: stopping a
// daemon must never reach the installed hive's.
func devDaemonFromEnv(logger zerolog.Logger, worktree string) (devDaemon, bool) {
	binary := os.Getenv(settings.EnvDaemonHivePath)
	dataDir := os.Getenv(settings.EnvHiveDataDir)
	if binary == "" || dataDir == "" {
		return devDaemon{}, false
	}
	inside := func(path string) bool {
		rel, err := filepath.Rel(worktree, path)
		return err == nil && filepath.IsAbs(path) && rel != "." && !strings.HasPrefix(rel, "..")
	}
	if !inside(binary) || !inside(dataDir) {
		logger.Warn().Str("binary", binary).Str("data_dir", dataDir).Msg("launch.env names a hive daemon outside this worktree; leaving it alone")
		return devDaemon{}, false
	}
	return devDaemon{worktree: worktree, binary: binary, dataDir: dataDir, logger: logger}, true
}

// build compiles this worktree's hive to the path the app starts its daemon
// from. Go's build cache makes an unchanged tree cheap.
func (d devDaemon) build() error {
	d.logger.Info().Str("binary", d.binary).Msg("building hive for the dev daemon")
	cmd := exec.Command("go", "build", "-o", d.binary, ".") //nolint:noctx // bounded by the build itself
	cmd.Dir = d.worktree
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("build hive for the dev daemon: %w", err)
	}
	return nil
}

// stop ends the instance's daemon and waits for its socket to stop answering,
// so the next app start spawns one from the fresh build rather than reaching
// one on its way out. No daemon running is not an error.
func (d devDaemon) stop() {
	ctx, cancel := context.WithTimeout(context.Background(), daemonStopTimeout)
	defer cancel()
	client := daemonclient.New(daemonclient.Options{Dialer: daemonclient.LocalDialer(d.dataDir), Logger: d.logger})
	defer client.HTTPClient().CloseIdleConnections()
	if _, err := client.Hello(ctx); err != nil {
		d.logger.Debug().Err(err).Msg("no dev daemon to stop")
		return
	}
	if err := client.Stop(ctx); err != nil {
		d.logger.Warn().Err(err).Msg("could not stop the dev daemon")
		return
	}
	for {
		client.HTTPClient().CloseIdleConnections()
		if _, err := client.Hello(ctx); err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				d.logger.Warn().Dur("timeout", daemonStopTimeout).Msg("the dev daemon did not stop in time")
			} else {
				d.logger.Info().Msg("stopped the dev daemon")
			}
			return
		}
		select {
		case <-ctx.Done():
		case <-time.After(daemonStopPoll):
		}
	}
}
