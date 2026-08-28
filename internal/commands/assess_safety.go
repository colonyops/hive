package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// socketPathResolver resolves the tmux socket path backing target's server.
// Injected so tests can fake the tmux round-trip; resolveTmuxSocketPath is
// the production implementation.
type socketPathResolver func(ctx context.Context, target string) (string, error)

type isolationDetector func() bool

const assessIsolationMarkerEnv = "HIVE_ASSESS_ISOLATED"

// runningInContainer recognizes the repository's supported `mise container`
// invocation, not arbitrary Docker environments that may mount host sockets.
func runningInContainer() bool {
	_, err := os.Stat("/.dockerenv")
	return supportedIsolationMarkers(err == nil, os.Getenv(assessIsolationMarkerEnv))
}

func supportedIsolationMarkers(dockerMarker bool, assessMarker string) bool {
	return dockerMarker && assessMarker == "1"
}

// resolveTmuxSocketPath asks tmux itself which socket target's server is
// listening on. Read-only: display-message never mutates the pane.
func resolveTmuxSocketPath(ctx context.Context, target string) (string, error) {
	out, err := exec.CommandContext(ctx, "tmux", "display-message", "-p", "-t", target, "#{socket_path}").Output()
	if err != nil {
		return "", fmt.Errorf("tmux display-message: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// ensureContainerSafe is the interlock `drive` and `scenario` both call
// before mutating a tmux pane. Socket names do not prove isolation: a host
// server may use any name, so the command fails closed unless it is running
// in the Docker environment created by `mise container`. allowHost is the
// deliberate, eyes-open escape hatch (--allow-host).
func ensureContainerSafe(ctx context.Context, target string, allowHost bool, resolve socketPathResolver, isolated isolationDetector) error {
	if allowHost {
		return nil
	}
	if isolated() {
		return nil
	}

	path, err := resolve(ctx, target)
	if err != nil {
		return fmt.Errorf(
			"refusing to mutate %s because hive cannot prove it is running inside `mise container`; "+
				"socket resolution also failed: %w. Pass --allow-host only for a deliberate host override",
			target, err,
		)
	}
	if path == "" {
		err = errors.New("tmux returned an empty socket path")
		return fmt.Errorf(
			"refusing to mutate %s because hive cannot prove it is running inside `mise container`: %w. "+
				"Pass --allow-host only for a deliberate host override",
			target, err,
		)
	}

	return fmt.Errorf(
		"refusing to mutate %s through tmux socket %s because socket names do not prove isolation; "+
			"run inside `mise container` or pass --allow-host for a deliberate host override",
		target, path,
	)
}
