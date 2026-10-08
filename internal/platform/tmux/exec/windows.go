package tmuxexec

import (
	"context"
	"fmt"
	"strings"

	"github.com/colonyops/hive/internal/domain/multiplexer"
)

// AddWindows adds windows to an existing tmux session.
func (c *Client) AddWindows(ctx context.Context, target multiplexer.Target, windows []multiplexer.WindowSpec) error {
	if err := target.ValidateSession(); err != nil {
		return err
	}
	name := target.Session
	c.suppressInteractiveHooks(ctx, name)
	var started []startedPane
	for _, window := range windows {
		windowPanes, err := c.createWindow(ctx, name, "", window)
		started = append(started, windowPanes...)
		if err != nil {
			return err
		}
	}
	if err := c.awaitStartup(ctx, name, started); err != nil {
		return err
	}
	for _, window := range windows {
		if !window.Focus {
			continue
		}
		if _, _, err := c.runner.Capture(ctx, "select-window", "-t", "="+name+":"+window.Name); err != nil {
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

func (c *Client) createWindow(ctx context.Context, sessionName, sessionDir string, window multiplexer.WindowSpec) ([]startedPane, error) {
	started, err := c.createInitialPane(ctx, []string{"new-window", "-t", "=" + sessionName + ":", "-n", window.Name}, sessionName, sessionDir, window)
	if err != nil {
		return nil, fmt.Errorf("tmux new-window %q: %w", window.Name, err)
	}
	c.tagPanesWithSession(ctx, "="+sessionName+":"+window.Name, sessionName)
	splitPanes, err := c.splitAdditionalPanes(ctx, sessionName, sessionDir, window)
	return append(started, splitPanes...), err
}

func (c *Client) createInitialPane(ctx context.Context, baseArgs []string, sessionName, sessionDir string, window multiplexer.WindowSpec) ([]startedPane, error) {
	command, dir := initialPane(window, sessionDir)
	args := append([]string(nil), baseArgs...)
	args = append(args, "-P", "-F", "#{pane_id}")
	if dir != "" {
		args = append(args, "-c", dir)
	}
	if command != "" {
		args = append(args, "--", "cat")
	}

	c.log.Debug().Strs("args", args).Msg("tmux " + baseArgs[0])
	stdout, _, err := c.runner.Capture(ctx, args...)
	if err != nil {
		return nil, err
	}
	return c.startPaneCommand(ctx, strings.TrimSpace(string(stdout)), "="+sessionName+":"+window.Name, window.Name, dir, command)
}

func (c *Client) splitAdditionalPanes(ctx context.Context, sessionName, sessionDir string, window multiplexer.WindowSpec) ([]startedPane, error) {
	target := "=" + sessionName + ":" + window.Name
	var started []startedPane
	for _, pane := range additionalPanes(window) {
		dir := pane.WorkingDirectory
		if dir == "" {
			dir = windowDir(window, sessionDir)
		}
		placeholder := ""
		if pane.Command != "" {
			placeholder = "cat"
		}
		stdout, _, err := c.runner.Capture(ctx, splitPaneArgs(target, pane, dir, placeholder)...)
		if err != nil {
			return started, fmt.Errorf("tmux split-window %q: %w", window.Name, err)
		}
		paneID := strings.TrimSpace(string(stdout))
		paneStarted, err := c.startPaneCommand(ctx, paneID, target, window.Name, dir, pane.Command)
		started = append(started, paneStarted...)
		if err != nil {
			return started, err
		}
		c.tagPanesWithSession(ctx, target, sessionName)
	}
	return started, nil
}

func (c *Client) startPaneCommand(ctx context.Context, paneID, fallbackTarget, windowName, dir, command string) ([]startedPane, error) {
	if command == "" {
		return nil, nil
	}
	target := paneID
	if target == "" {
		target = fallbackTarget
	}
	if _, _, err := c.runner.Capture(ctx, "set-option", "-w", "-t", target, "remain-on-exit", "on"); err != nil {
		return nil, fmt.Errorf("tmux set-option remain-on-exit: %w", err)
	}

	args := []string{"respawn-pane", "-k", "-t", target}
	if dir != "" {
		args = append(args, "-c", dir)
	}
	args = append(args, "--", "sh", "-c", command)
	if _, _, err := c.runner.Capture(ctx, args...); err != nil {
		return nil, fmt.Errorf("tmux respawn-pane: %w", err)
	}
	return watchPane(paneID, windowName, command), nil
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
	args := []string{"split-window", "-t", target, "-P", "-F", "#{pane_id}"}
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
