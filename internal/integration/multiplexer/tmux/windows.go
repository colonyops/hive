package tmux

import (
	"context"
	"fmt"

	"github.com/colonyops/hive/internal/domain/multiplexer"
)

// AddWindows adds windows to an existing tmux session.
func (c *Client) AddWindows(ctx context.Context, target multiplexer.Target, windows []multiplexer.WindowSpec) error {
	name, err := renderSessionTarget(target)
	if err != nil {
		return err
	}
	c.suppressInteractiveHooks(ctx, name)
	for _, window := range windows {
		if err := c.createWindow(ctx, name, "", window); err != nil {
			return err
		}
	}
	for _, window := range windows {
		if !window.Focus {
			continue
		}
		if _, _, err := c.runner.Capture(ctx, "select-window", "-t", name+":"+window.Name); err != nil {
			return fmt.Errorf("tmux select-window %q: %w", window.Name, err)
		}
		break
	}
	return nil
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

func (c *Client) createWindow(ctx context.Context, sessionName, sessionDir string, window multiplexer.WindowSpec) error {
	args := []string{"new-window", "-t", sessionName, "-n", window.Name}
	args = appendInitialPaneArgs(args, window, sessionDir)
	if _, _, err := c.runner.Capture(ctx, args...); err != nil {
		return fmt.Errorf("tmux new-window %q: %w", window.Name, err)
	}
	c.tagPanesWithSession(ctx, sessionName+":"+window.Name, sessionName)
	return c.splitAdditionalPanes(ctx, sessionName, sessionDir, window)
}

func (c *Client) splitAdditionalPanes(ctx context.Context, sessionName, sessionDir string, window multiplexer.WindowSpec) error {
	target := sessionName + ":" + window.Name
	for _, pane := range additionalPanes(window) {
		args := splitPaneArgs(target, pane, windowDir(window, sessionDir))
		if _, _, err := c.runner.Capture(ctx, args...); err != nil {
			return fmt.Errorf("tmux split-window %q: %w", window.Name, err)
		}
		c.tagPanesWithSession(ctx, target, sessionName)
	}
	return nil
}

func appendInitialPaneArgs(args []string, window multiplexer.WindowSpec, sessionDir string) []string {
	command := window.Command
	dir := windowDir(window, sessionDir)
	if len(window.Panes) > 0 {
		command = window.Panes[0].Command
		if window.Panes[0].WorkingDirectory != "" {
			dir = window.Panes[0].WorkingDirectory
		}
	}
	if dir != "" {
		args = append(args, "-c", dir)
	}
	if command != "" {
		args = append(args, "--", "sh", "-c", command)
	}
	return args
}

func splitPaneArgs(target string, pane multiplexer.PaneSpec, fallbackDir string) []string {
	args := []string{"split-window", "-t", target}
	if pane.Split == multiplexer.SplitHorizontal {
		args = append(args, "-h")
	} else {
		args = append(args, "-v")
	}
	if pane.Size != "" {
		args = append(args, "-l", pane.Size)
	}
	dir := pane.WorkingDirectory
	if dir == "" {
		dir = fallbackDir
	}
	if dir != "" {
		args = append(args, "-c", dir)
	}
	if pane.Command != "" {
		args = append(args, "--", "sh", "-c", pane.Command)
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
