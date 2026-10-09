package tmuxexec

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// LaunchPhase identifies the phase of a bounded tmux launch.
// ENUM(allocating, launching, observing, finalizing, committed, rolling_back, failed, cleanup_incomplete)
type LaunchPhase string

// PaneLaunchState identifies a pane's observed startup outcome.
// ENUM(allocated, spawned, running, completed, exit_pending, failed, lost)
type PaneLaunchState string

const launchCleanupTimeout = 2 * time.Second

type optionLease struct {
	name     string
	value    string
	explicit bool
	optional bool
}

type paneOption struct {
	name  string
	value string
	// optional options may not exist in older tmux; -q makes them no-ops there.
	optional bool
}

// remain-on-exit-format arrived in tmux 3.3. On 3.2 the dead pane keeps
// tmux's own "Pane is dead" line, which only adds a line to the report.
var launchPaneOptions = []paneOption{
	{name: "remain-on-exit-format", value: "", optional: true},
	{name: "remain-on-exit", value: "on"},
}

type launchPane struct {
	id        string
	window    string
	command   string
	dir       string
	state     PaneLaunchState
	deadline  time.Time
	retention string
	leases    []optionLease
	removed   bool
}

type launchWindow struct {
	id      string
	name    string
	focus   bool
	panes   []*launchPane
	removed bool
}

type launch struct {
	client      *Client
	name        string
	sessionID   string
	ownsSession bool
	phase       LaunchPhase
	windows     []*launchWindow
}

// LaunchError preserves the failed phase and all command and cleanup causes.
type LaunchError struct {
	Session string
	Phase   LaunchPhase
	// AddedWindows is true when the launch added windows to a session that
	// already existed, so the session itself did not fail.
	AddedWindows bool
	Err          error
}

func (e *LaunchError) Error() string {
	if e.AddedWindows {
		return fmt.Sprintf("new windows in tmux session %q failed to start: %v", e.Session, e.Err)
	}
	return fmt.Sprintf("tmux session %q failed to start: %v", e.Session, e.Err)
}

func (e *LaunchError) Unwrap() error { return e.Err }

// CleanupError reports resources that could not be removed after a failed launch.
type CleanupError struct {
	Resources []string
	Err       error
}

func (e *CleanupError) Error() string {
	return fmt.Sprintf("cleanup did not complete for tmux resources %s: %v", strings.Join(e.Resources, ", "), e.Err)
}

func (e *CleanupError) Unwrap() error { return e.Err }

func (l *launch) fail(ctx context.Context, err error) error {
	failedPhase := l.phase
	l.phase = LaunchPhaseRollingBack
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), launchCleanupTimeout)
	defer cancel()
	var failures []error
	var resources []string
	remove := func(command, id string, windows []*launchWindow) {
		if _, _, cleanupErr := l.client.runner.Capture(cleanupCtx, command, "-t", id); cleanupErr != nil {
			if !l.resourcePresent(cleanupCtx, id) {
				return
			}
			resources = append(resources, id)
			failures = append(failures, cleanupErr)
			for _, window := range windows {
				for _, pane := range window.panes {
					if !pane.removed {
						if restoreErr := l.restorePane(cleanupCtx, pane); restoreErr != nil {
							failures = append(failures, restoreErr)
						}
					}
				}
			}
		}
	}
	if l.ownsSession && l.sessionID != "" {
		if !l.allRemoved() {
			remove("kill-session", l.sessionID, l.windows)
		}
	} else {
		for _, window := range l.windows {
			if !window.removed {
				remove("kill-window", window.id, []*launchWindow{window})
			}
		}
	}
	l.phase = LaunchPhaseFailed
	if len(failures) > 0 {
		l.phase = LaunchPhaseCleanupIncomplete
		err = errors.Join(err, &CleanupError{Resources: resources, Err: errors.Join(failures...)})
	}
	return &LaunchError{Session: l.name, Phase: failedPhase, AddedWindows: !l.ownsSession, Err: err}
}

func (l *launch) allRemoved() bool {
	for _, window := range l.windows {
		if !window.removed {
			return false
		}
	}
	return true
}

func (l *launch) start(ctx context.Context) error {
	l.phase = LaunchPhaseLaunching
	for _, window := range l.windows {
		for _, pane := range window.panes {
			if err := l.arm(ctx, pane); err != nil {
				return err
			}
			if pane.command == "" {
				pane.state = PaneLaunchStateSpawned
				pane.deadline = time.Now().Add(l.client.startupGrace)
				continue
			}
			args := []string{"respawn-pane", "-k", "-t", pane.id}
			if pane.dir != "" {
				args = append(args, "-c", pane.dir)
			}
			args = append(args, "--", "sh", "-c", pane.command)
			if _, _, err := l.client.runner.Capture(ctx, args...); err != nil {
				return fmt.Errorf("tmux respawn-pane in window %q: %w", window.name, err)
			}
			pane.state = PaneLaunchStateSpawned
			pane.deadline = time.Now().Add(l.client.startupGrace)
		}
	}
	l.phase = LaunchPhaseObserving
	return l.observe(ctx)
}

func (l *launch) arm(ctx context.Context, pane *launchPane) error {
	stdout, _, err := l.client.runner.Capture(ctx, "show-options", "-p", "-A", "-v", "-t", pane.id, "remain-on-exit")
	if err != nil {
		return fmt.Errorf("read pane retention: %w", err)
	}
	pane.retention = strings.TrimSpace(string(stdout))
	for _, option := range launchPaneOptions {
		previous, _, err := l.client.runner.Capture(ctx, optionArgs(option.optional, "show-options", "-p", "-v", "-t", pane.id, option.name)...)
		if err != nil {
			return fmt.Errorf("read pane option %s: %w", option.name, err)
		}
		pane.leases = append(pane.leases, optionLease{name: option.name, value: strings.TrimSuffix(string(previous), "\n"), explicit: len(previous) > 0, optional: option.optional})
		if _, _, err := l.client.runner.Capture(ctx, optionArgs(option.optional, "set-option", "-p", "-t", pane.id, option.name, option.value)...); err != nil {
			return fmt.Errorf("set pane option %s: %w", option.name, err)
		}
	}
	return nil
}

func optionArgs(optional bool, command string, args ...string) []string {
	if optional {
		return append([]string{command, "-q"}, args...)
	}
	return append([]string{command}, args...)
}

func (l *launch) resourcePresent(ctx context.Context, id string) bool {
	args := []string{"list-sessions", "-F", "#{session_id}"}
	if strings.HasPrefix(id, "@") {
		args = []string{"list-windows", "-a", "-F", "#{window_id}"}
	}
	stdout, _, err := l.client.runner.Capture(ctx, args...)
	if err != nil {
		return true
	}
	return slices.Contains(strings.Fields(string(stdout)), id)
}

func (l *launch) restorePane(ctx context.Context, pane *launchPane) error {
	var failures []error
	for _, lease := range pane.leases {
		args := []string{"-p", "-t", pane.id}
		if !lease.explicit {
			args = append(args, "-u")
		}
		args = append(args, lease.name)
		if lease.explicit {
			args = append(args, lease.value)
		}
		if _, _, err := l.client.runner.Capture(ctx, optionArgs(lease.optional, "set-option", args...)...); err != nil {
			failures = append(failures, fmt.Errorf("restore pane option %s: %w", lease.name, err))
		}
	}
	return errors.Join(failures...)
}

// finalize observes once more while exits are still retained, then commits.
// Pane options are restored only after every check that can fail the launch:
// once retention is off, tmux removes a pane that exits, and a later check
// would mistake that ordinary exit for a lost pane and roll back healthy work.
func (l *launch) finalize(ctx context.Context) (bool, error) {
	l.phase = LaunchPhaseFinalizing
	if err := l.observe(ctx); err != nil {
		return false, err
	}
	var focus *launchWindow
	for _, window := range l.windows {
		remaining := 0
		for _, pane := range window.panes {
			if pane.state == PaneLaunchStateCompleted && pane.retention != "on" {
				if _, _, err := l.client.runner.Capture(ctx, "kill-pane", "-t", pane.id); err != nil {
					return false, fmt.Errorf("remove completed pane: %w", err)
				}
				pane.removed = true
			} else {
				remaining++
			}
		}
		window.removed = remaining == 0
		// A new session opens on its first window; added windows end on the
		// last one, as plain new-window would. An explicit focus wins either way.
		if remaining > 0 && (focus == nil || !focus.focus && (window.focus || !l.ownsSession)) {
			focus = window
		}
	}
	if focus != nil {
		if _, _, err := l.client.runner.Capture(ctx, "select-window", "-t", focus.id); err != nil {
			return false, fmt.Errorf("tmux select-window: %w", err)
		}
	}
	for _, window := range l.windows {
		for _, pane := range window.panes {
			if !pane.removed {
				if err := l.restorePane(ctx, pane); err != nil {
					return false, err
				}
			}
		}
	}
	for _, window := range l.windows {
		if !window.removed {
			if err := l.clearMarker(ctx, window.id, "-w"); err != nil {
				return false, fmt.Errorf("clear window launch marker: %w", err)
			}
		}
	}
	if l.ownsSession && !l.allRemoved() {
		if err := l.clearMarker(ctx, l.sessionID); err != nil {
			return false, fmt.Errorf("clear session launch marker: %w", err)
		}
	}
	l.phase = LaunchPhaseCommitted
	return focus == nil, nil
}

// clearMarker runs after pane options are restored, so the resource may have
// closed on its own in between. A resource that is gone needs no marker.
func (l *launch) clearMarker(ctx context.Context, id string, flags ...string) error {
	args := append([]string{"set-option"}, flags...)
	args = append(args, "-u", "-t", id, launchMarker)
	if _, _, err := l.client.runner.Capture(ctx, args...); err != nil && l.resourcePresent(ctx, id) {
		return err
	}
	return nil
}
