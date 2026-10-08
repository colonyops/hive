package tmuxexec

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/colonyops/hive/internal/domain/multiplexer"
)

// HasSession reports whether a tmux session exists.
func (c *Client) HasSession(ctx context.Context, target multiplexer.Target) (bool, error) {
	rendered, err := renderSessionTarget(target)
	if err != nil {
		return false, err
	}
	_, _, err = c.runner.Capture(ctx, "has-session", "-t", rendered)
	if err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		return false, nil
	}
	return true, nil
}

// CreateSession creates a tmux session from a declarative specification.
func (c *Client) CreateSession(ctx context.Context, spec multiplexer.SessionSpec) error {
	if err := spec.Target.ValidateSession(); err != nil {
		return err
	}
	name := spec.Target.Session
	if len(spec.Windows) == 0 {
		return fmt.Errorf("tmux: at least one window is required")
	}

	first := spec.Windows[0]
	started, err := c.createInitialPane(ctx, []string{"new-session", "-d", "-s", name, "-n", first.Name}, name, spec.WorkingDirectory, first)
	if err != nil {
		return fmt.Errorf("tmux new-session: %w", err)
	}

	partial := true
	defer func() {
		if !partial {
			return
		}
		if _, _, cleanupErr := c.runner.Capture(context.WithoutCancel(ctx), "kill-session", "-t", "="+name); cleanupErr != nil {
			c.log.Debug().Err(cleanupErr).Str("session", name).Msg("failed to clean up partial tmux session")
		}
	}()

	c.tagPanesWithSession(ctx, "="+name+":", name)
	c.suppressInteractiveHooks(ctx, name)
	splitPanes, err := c.splitAdditionalPanes(ctx, name, spec.WorkingDirectory, first)
	started = append(started, splitPanes...)
	if err != nil {
		return err
	}
	for _, window := range spec.Windows[1:] {
		windowPanes, err := c.createWindow(ctx, name, spec.WorkingDirectory, window)
		started = append(started, windowPanes...)
		if err != nil {
			return err
		}
	}
	if err := c.awaitStartup(ctx, name, started); err != nil {
		return err
	}

	focusName := first.Name
	for _, window := range spec.Windows {
		if window.Focus {
			focusName = window.Name
			break
		}
	}
	if _, _, err := c.runner.Capture(ctx, "select-window", "-t", "="+name+":"+focusName); err != nil {
		return fmt.Errorf("tmux select-window: %w", err)
	}
	partial = false
	if !spec.Background {
		return c.AttachOrSwitch(ctx, spec.Target, multiplexer.AttachStreams{})
	}
	return nil
}

// OpenSession creates a missing session or opens an existing one. For an
// existing session, selection names the window or pane to show on entry.
func (c *Client) OpenSession(ctx context.Context, spec multiplexer.SessionSpec, selection multiplexer.Target) error {
	exists, err := c.HasSession(ctx, spec.Target)
	if err != nil {
		return err
	}
	if !exists {
		return c.CreateSession(ctx, spec)
	}
	if spec.Background {
		return nil
	}
	selection.Session = spec.Target.Session
	return c.AttachOrSwitch(ctx, selection, multiplexer.AttachStreams{})
}

// AttachOrSwitch switches the active client inside tmux or attaches outside tmux.
// Window and pane selection is best-effort and ordered so the selected target is
// visible on entry: attach-session blocks until detach, so selection runs before
// it, while switch-client returns immediately, so selection runs after it.
func (c *Client) AttachOrSwitch(ctx context.Context, target multiplexer.Target, streams multiplexer.AttachStreams) error {
	sessionTarget := multiplexer.Target{Session: target.Session}
	rendered, err := renderSessionTarget(sessionTarget)
	if err != nil {
		return err
	}
	if c.insideTmux() {
		if _, _, err := c.runner.Capture(ctx, "switch-client", "-t", rendered); err != nil {
			return fmt.Errorf("tmux switch-client: %w", err)
		}
		c.selectTarget(ctx, target)
		return nil
	}
	c.selectTarget(ctx, target)
	if streams.Stdin == nil {
		streams.Stdin = os.Stdin
	}
	if streams.Stdout == nil {
		streams.Stdout = os.Stdout
	}
	if streams.Stderr == nil {
		streams.Stderr = os.Stderr
	}
	if err := c.runner.Interactive(ctx, streams, "attach-session", "-t", rendered); err != nil {
		return fmt.Errorf("tmux attach-session: %w", err)
	}
	return nil
}

// RenameSession renames a tmux session.
func (c *Client) RenameSession(ctx context.Context, target multiplexer.Target, newName string) error {
	oldName, err := renderSessionTarget(target)
	if err != nil {
		return err
	}
	newTarget := multiplexer.Target{Session: newName}
	if err := newTarget.ValidateSession(); err != nil {
		return err
	}
	if _, _, err := c.runner.Capture(ctx, "rename-session", "-t", oldName, newName); err != nil {
		return fmt.Errorf("tmux rename-session %q to %q: %w", oldName, newName, err)
	}
	return nil
}

// KillSession kills a tmux session.
func (c *Client) KillSession(ctx context.Context, target multiplexer.Target) error {
	rendered, err := renderSessionTarget(target)
	if err != nil {
		return err
	}
	if _, _, err := c.runner.Capture(ctx, "kill-session", "-t", rendered); err != nil {
		return fmt.Errorf("tmux kill-session %q: %w", rendered, err)
	}
	return nil
}

func (c *Client) insideTmux() bool { return strings.TrimSpace(c.getenv("TMUX")) != "" }

// selectTarget is best-effort: the window or pane may have been renamed or
// closed since the caller resolved it, and attaching to the current window is
// the right fallback.
func (c *Client) selectTarget(ctx context.Context, target multiplexer.Target) {
	if target.Session == "" {
		return
	}
	if target.Window == "" && target.Pane != "" {
		// Legacy callers pass a native %N pane ID without a window. select-pane
		// alone does not move the client to the pane's window, so resolve the
		// window first and select it.
		stdout, _, err := c.runner.Capture(ctx, "display-message", "-p", "-t", target.Pane, "#{session_name}:#{window_index}")
		if err == nil {
			if window := string(bytes.TrimSpace(stdout)); window != "" {
				_, _, _ = c.runner.Capture(ctx, "select-window", "-t", "="+window)
			}
		}
		_, _, _ = c.runner.Capture(ctx, "select-pane", "-t", target.Pane)
		return
	}
	if target.Window == "" {
		return
	}
	windowTarget := multiplexer.Target{Session: target.Session, Window: target.Window}
	renderedWindow, err := renderWindowTarget(windowTarget)
	if err != nil {
		return
	}
	if _, _, err := c.runner.Capture(ctx, "select-window", "-t", renderedWindow); err != nil {
		c.log.Debug().Err(err).Str("target", renderedWindow).Msg("failed to select tmux window")
		return
	}
	if target.Pane == "" {
		return
	}
	renderedPane, err := renderPaneTarget(target)
	if err != nil {
		return
	}
	if _, _, err := c.runner.Capture(ctx, "select-pane", "-t", renderedPane); err != nil {
		c.log.Debug().Err(err).Str("target", renderedPane).Msg("failed to select tmux pane")
	}
}

// tagPanesWithSession sets @hive-session on the active pane so list-panes can
// identify hive-managed panes after a user renames the tmux session. Tagging is
// best-effort.
func (c *Client) tagPanesWithSession(ctx context.Context, target, slug string) {
	if _, _, err := c.runner.Capture(ctx, "set-option", "-p", "-t", target, "@hive-session", slug); err != nil {
		c.log.Debug().Err(err).Str("target", target).Msg("failed to tag pane with @hive-session")
	}
}

// suppressInteractiveHooks overrides global hooks that run interactive commands
// such as command-prompt. Those hooks block the tmux server waiting for input
// that never arrives when windows are created programmatically. Failure is
// non-fatal; the worst case is the old blocking behaviour.
func (c *Client) suppressInteractiveHooks(ctx context.Context, session string) {
	for _, hook := range []string{"after-new-window", "after-split-window"} {
		if _, _, err := c.runner.Capture(ctx, "set-hook", "-t", "="+session, hook, ""); err != nil {
			c.log.Debug().Err(err).Str("hook", hook).Msg("failed to suppress hook")
		}
	}
}
