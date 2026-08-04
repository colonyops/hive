package commands

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// socketPathResolver resolves the tmux socket path backing target's server.
// Injected so tests can fake the tmux round-trip; resolveTmuxSocketPath is
// the production implementation.
type socketPathResolver func(ctx context.Context, target string) (string, error)

// isHostDefaultSocket reports whether path is tmux's default per-user socket
// — created implicitly by any bare `tmux` invocation with no -L/-S flag, of
// the form /tmp/tmux-<uid>/default. It is a pure predicate on the basename so
// it can be unit-tested without touching tmux at all.
func isHostDefaultSocket(path string) bool {
	return filepath.Base(path) == "default"
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
// before sending a single keystroke: these commands send real input to a
// tmux pane, and running that against the operator's own host tmux server
// has crashed dev environments before (see CLAUDE.md's host-tmux rule) — so
// by default they refuse to run anywhere but an isolated socket (in
// practice, inside `mise container`). allowHost is the deliberate,
// eyes-open escape hatch (--allow-host). resolve is injected so the refusal
// path is unit-testable without a real tmux server.
func ensureContainerSafe(ctx context.Context, target string, allowHost bool, resolve socketPathResolver) error {
	if allowHost {
		return nil
	}

	path, err := resolve(ctx, target)
	if err != nil {
		return fmt.Errorf("resolving tmux socket for %s (safety check): %w", target, err)
	}

	if isHostDefaultSocket(path) {
		return fmt.Errorf(
			"refusing to send input to %s: it resolves to the host's default tmux socket (%s). "+
				"drive and scenario send real keystrokes to a tmux pane and must only run inside "+
				"`mise container` — host tmux spawning has crashed dev environments before (see CLAUDE.md). "+
				"Pass --allow-host if you are deliberately overriding this",
			target, path,
		)
	}

	return nil
}
