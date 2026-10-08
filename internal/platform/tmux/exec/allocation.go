package tmuxexec

import (
	"context"
	"fmt"
	"strings"

	"github.com/colonyops/hive/internal/domain/multiplexer"
)

const allocationFormat = "#{session_id} #{window_id} #{pane_id}"

// LaunchPendingOption marks resources that have not committed a Hive launch.
const LaunchPendingOption = "@hive-launch-pending"

const launchMarker = LaunchPendingOption

func (l *launch) allocateWindow(ctx context.Context, sessionDir string, spec multiplexer.WindowSpec) error {
	command, dir := initialPane(spec, sessionDir)
	args := []string{"new-window", "-d", "-t", "=" + l.name + ":", "-n", spec.Name}
	if l.ownsSession && l.sessionID == "" {
		args = []string{"new-session", "-d", "-s", l.name, "-n", spec.Name}
	}
	args = append(args, "-P", "-F", allocationFormat)
	if dir != "" {
		args = append(args, "-c", dir)
	}
	if command != "" {
		args = append(args, "--", "cat")
	}
	if args[0] == "new-session" {
		args = append(args, ";", "set-option", launchMarker, "1")
	}
	stdout, _, commandErr := l.client.runner.Capture(ctx, args...)
	ids := strings.Fields(string(stdout))
	if !validAllocation(ids) {
		if commandErr != nil {
			return fmt.Errorf("tmux %s: %w", args[0], commandErr)
		}
		return fmt.Errorf("tmux %s returned no valid resource IDs", args[0])
	}
	l.sessionID = ids[0]
	window := &launchWindow{id: ids[1], name: spec.Name, focus: spec.Focus}
	window.panes = append(window.panes, &launchPane{id: ids[2], window: spec.Name, command: command, dir: dir, state: PaneLaunchStateAllocated})
	l.windows = append(l.windows, window)
	if commandErr != nil {
		return fmt.Errorf("tmux %s: %w", args[0], commandErr)
	}
	if _, _, err := l.client.runner.Capture(ctx, "set-option", "-w", "-t", window.id, launchMarker, "1"); err != nil {
		return fmt.Errorf("mark allocated window: %w", err)
	}
	l.client.tagPanesWithSession(ctx, ids[2], l.name)
	l.client.suppressInteractiveHooks(ctx, l.name)
	for _, paneSpec := range additionalPanes(spec) {
		paneDir := paneSpec.WorkingDirectory
		if paneDir == "" {
			paneDir = windowDir(spec, sessionDir)
		}
		placeholder := ""
		if paneSpec.Command != "" {
			placeholder = "cat"
		}
		stdout, _, commandErr = l.client.runner.Capture(ctx, splitPaneArgs(window.id, paneSpec, paneDir, placeholder)...)
		ids = strings.Fields(string(stdout))
		if !validAllocation(ids) {
			if commandErr != nil {
				return fmt.Errorf("tmux split-window: %w", commandErr)
			}
			return fmt.Errorf("tmux split-window returned no valid resource IDs")
		}
		window.panes = append(window.panes, &launchPane{id: ids[2], window: spec.Name, command: paneSpec.Command, dir: paneDir, state: PaneLaunchStateAllocated})
		if commandErr != nil {
			return fmt.Errorf("tmux split-window: %w", commandErr)
		}
		l.client.tagPanesWithSession(ctx, ids[2], l.name)
	}
	return nil
}

func validAllocation(ids []string) bool {
	return len(ids) == 3 && validNativeID(ids[0], '$') && validNativeID(ids[1], '@') && validNativeID(ids[2], '%')
}

func validNativeID(id string, prefix byte) bool {
	if len(id) < 2 || id[0] != prefix {
		return false
	}
	for _, digit := range id[1:] {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}
