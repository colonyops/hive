package tmuxexec

import (
	"context"
	"fmt"

	"github.com/colonyops/hive/internal/domain/multiplexer"
)

// AddWindows adds windows to an existing tmux session.
func (c *Client) AddWindows(ctx context.Context, target multiplexer.Target, windows []multiplexer.WindowSpec) error {
	if err := target.ValidateSession(); err != nil {
		return err
	}
	return c.withLaunchLock(ctx, target.Session, func() error {
		exists, err := c.recoverLaunch(ctx, target.Session)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("tmux session %q had an interrupted launch and must be restarted", target.Session)
		}
		l := &launch{client: c, name: target.Session, phase: LaunchPhaseAllocating}
		c.suppressInteractiveHooks(ctx, target.Session)
		for _, window := range windows {
			if err := l.allocateWindow(ctx, "", window); err != nil {
				return l.fail(ctx, err)
			}
		}
		if err := l.start(ctx); err != nil {
			return l.fail(ctx, err)
		}
		if _, err := l.finalize(ctx); err != nil {
			return l.fail(ctx, err)
		}
		return nil
	})
}

// KillWindow kills one qualified tmux window.
func (c *Client) KillWindow(ctx context.Context, target multiplexer.Target) error {
	rendered, err := renderWindowTarget(target)
	if err != nil {
		return err
	}
	if _, _, err := c.runner.Capture(ctx, "kill-window", "-t", rendered); err != nil {
		return fmt.Errorf("tmux kill-window %q: %w", rendered, err)
	}
	return nil
}

func initialPane(window multiplexer.WindowSpec, sessionDir string) (command, dir string) {
	command = window.Command
	dir = windowDir(window, sessionDir)
	if len(window.Panes) > 0 {
		command = window.Panes[0].Command
		if window.Panes[0].WorkingDirectory != "" {
			dir = window.Panes[0].WorkingDirectory
		}
	}
	return command, dir
}

func splitPaneArgs(target string, pane multiplexer.PaneSpec, dir, command string) []string {
	args := []string{"split-window", "-d", "-t", target, "-P", "-F", allocationFormat}
	if pane.Split == multiplexer.SplitHorizontal {
		args = append(args, "-h")
	} else {
		args = append(args, "-v")
	}
	if pane.Size != "" {
		args = append(args, "-l", pane.Size)
	}
	if dir != "" {
		args = append(args, "-c", dir)
	}
	if command != "" {
		args = append(args, "--", "sh", "-c", command)
	}
	return args
}

func additionalPanes(window multiplexer.WindowSpec) []multiplexer.PaneSpec {
	if len(window.Panes) <= 1 {
		return nil
	}
	return window.Panes[1:]
}

func windowDir(window multiplexer.WindowSpec, sessionDir string) string {
	if window.WorkingDirectory != "" {
		return window.WorkingDirectory
	}
	return sessionDir
}
