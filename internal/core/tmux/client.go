// Package tmux provides a Go-native tmux session client that creates sessions
// from declarative window definitions.
package tmux

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/colonyops/hive/pkg/executil"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// RenderedPane is a fully-resolved tmux pane definition (no templates).
type RenderedPane struct {
	Command string // Command to run (empty = default shell)
	Dir     string // Working directory (empty = window/session default)
	Size    string // Pane size passed to tmux -l (empty = tmux default)
	Split   string // Split direction: horizontal or vertical (default vertical)
}

// RenderedWindow is a fully-resolved tmux window definition (no templates).
type RenderedWindow struct {
	Name    string         // Window name
	Command string         // Command to run (empty = default shell); ignored when Panes is non-empty
	Dir     string         // Working directory (empty = session default)
	Focus   bool           // Select this window after creation
	Panes   []RenderedPane // Panes to create in this window; mutually exclusive with Command
}

// Client creates and manages tmux sessions from window definitions.
type Client struct {
	exec executil.Executor
	log  zerolog.Logger
}

// New creates a Client with the given executor and logger.
func New(exec executil.Executor, log zerolog.Logger) *Client {
	return &Client{exec: exec, log: log}
}

// HasSession checks whether a tmux session with the given name exists.
func (c *Client) HasSession(ctx context.Context, name string) bool {
	_, err := c.exec.Run(ctx, "tmux", "has-session", "-t", name)
	return err == nil
}

// CreateSession creates a tmux session with the given windows.
// The first window is created via new-session; additional windows via new-window.
// If background is true, the session is created detached.
func (c *Client) CreateSession(ctx context.Context, name, workDir string, windows []RenderedWindow, background bool) error {
	if len(windows) == 0 {
		return fmt.Errorf("tmux: at least one window is required")
	}

	// Create session with the first window.
	first := windows[0]
	args := []string{"new-session", "-d", "-s", name, "-n", first.Name}
	args = appendInitialPaneArgs(args, first, workDir)

	c.log.Debug().Strs("args", args).Msg("tmux new-session")
	if out, err := c.exec.Run(ctx, "tmux", args...); err != nil {
		return fmt.Errorf("tmux new-session: %w; output: %s", err, strings.TrimSpace(string(out)))
	}

	// Tag the initial pane for hive-managed pane identification.
	c.tagPanesWithSession(ctx, name, name)

	// Suppress interactive hooks (e.g. after-new-window command-prompt) that
	// block the tmux server waiting for input that will never arrive when
	// windows are created programmatically.
	c.suppressInteractiveHooks(ctx, name)

	if err := c.splitAdditionalPanes(ctx, name, workDir, first); err != nil {
		_, _ = c.exec.Run(ctx, "tmux", "kill-session", "-t", name)
		return err
	}

	// Create additional windows. On failure, kill the partial session.
	for _, w := range windows[1:] {
		if err := c.createWindow(ctx, name, workDir, w); err != nil {
			_, _ = c.exec.Run(ctx, "tmux", "kill-session", "-t", name)
			return err
		}
	}

	// Select the focused window (default to first).
	focusName := windows[0].Name
	for _, w := range windows {
		if w.Focus {
			focusName = w.Name
			break
		}
	}
	selectArgs := []string{"select-window", "-t", name + ":" + focusName}
	c.log.Debug().Strs("args", selectArgs).Msg("tmux select-window")
	if out, err := c.exec.Run(ctx, "tmux", selectArgs...); err != nil {
		return fmt.Errorf("tmux select-window: %w; output: %s", err, strings.TrimSpace(string(out)))
	}

	if !background {
		return c.AttachOrSwitch(ctx, name)
	}
	return nil
}

// AddWindows adds windows to an existing tmux session.
// If any window has Focus set, that window is selected after all windows are created.
func (c *Client) AddWindows(ctx context.Context, name, workDir string, windows []RenderedWindow) error {
	c.suppressInteractiveHooks(ctx, name)
	for _, w := range windows {
		if err := c.createWindow(ctx, name, workDir, w); err != nil {
			return err
		}
	}
	for _, w := range windows {
		if w.Focus {
			if _, err := c.exec.Run(ctx, "tmux", "select-window", "-t", name+":"+w.Name); err != nil {
				return fmt.Errorf("tmux select-window %q: %w", w.Name, err)
			}
			break
		}
	}
	return nil
}

// AttachOrSwitch connects to an existing tmux session.
// Inside tmux it uses switch-client; outside it uses attach-session.
func (c *Client) AttachOrSwitch(ctx context.Context, name string) error {
	if insideTmux() {
		_, err := c.exec.Run(ctx, "tmux", "switch-client", "-t", name)
		if err != nil {
			return fmt.Errorf("tmux switch-client: %w", err)
		}
		return nil
	}

	_, err := c.exec.Run(ctx, "tmux", "attach-session", "-t", name)
	if err != nil {
		return fmt.Errorf("tmux attach-session: %w", err)
	}
	return nil
}

// OpenSession creates a session if it doesn't exist, or attaches to it.
// If targetWindow is non-empty and the session already exists, select that legacy tmux target (window or pane).
func (c *Client) OpenSession(ctx context.Context, name, workDir string, windows []RenderedWindow, background bool, targetWindow string) error {
	if c.HasSession(ctx, name) {
		if background {
			return nil
		}
		if insideTmux() {
			if err := c.AttachOrSwitch(ctx, name); err != nil {
				return err
			}
			c.selectTarget(ctx, name, targetWindow)
			return nil
		}
		c.selectTarget(ctx, name, targetWindow)
		return c.AttachOrSwitch(ctx, name)
	}
	return c.CreateSession(ctx, name, workDir, windows, background)
}

func (c *Client) selectTarget(ctx context.Context, sessionName, target string) {
	if target == "" {
		return
	}
	if strings.HasPrefix(target, "%") {
		c.selectPaneTarget(ctx, target)
		return
	}
	// Best-effort: window may not exist if config changed since session was created.
	// Failure is expected (e.g., window renamed/closed) — attach to current window instead.
	_, _ = c.exec.Run(ctx, "tmux", "select-window", "-t", sessionName+":"+target)
}

func (c *Client) selectPaneTarget(ctx context.Context, paneID string) {
	// select-pane alone does not move the client/session to the pane's window.
	// Resolve the pane's window first, then select the pane inside it.
	out, err := c.exec.Run(ctx, "tmux", "display-message", "-p", "-t", paneID, "#{session_name}:#{window_index}")
	if err == nil {
		if windowTarget := strings.TrimSpace(string(out)); windowTarget != "" {
			_, _ = c.exec.Run(ctx, "tmux", "select-window", "-t", windowTarget)
		}
	}
	_, _ = c.exec.Run(ctx, "tmux", "select-pane", "-t", paneID)
}

// tagPanesWithSession sets @hive-session on the active pane so list-panes can
// identify hive-managed panes. Errors are non-fatal — tagging is best-effort.
func (c *Client) tagPanesWithSession(ctx context.Context, sessionTarget, slug string) {
	if _, err := c.exec.Run(ctx, "tmux", "set-option", "-p", "-t", sessionTarget, "@hive-session", slug); err != nil {
		c.log.Debug().Err(err).Str("target", sessionTarget).Msg("failed to tag pane with @hive-session")
	}
}

// suppressInteractiveHooks sets session-level overrides to neutralise global
// hooks that run interactive commands (e.g. command-prompt). These hooks block
// the tmux server waiting for user input that never arrives when windows are
// created programmatically. Errors are logged but not fatal — the worst case
// is the old blocking behaviour.
func (c *Client) suppressInteractiveHooks(ctx context.Context, session string) {
	hooks := []string{"after-new-window", "after-split-window"}
	for _, h := range hooks {
		if _, err := c.exec.Run(ctx, "tmux", "set-hook", "-t", session, h, ""); err != nil {
			c.log.Debug().Err(err).Str("hook", h).Msg("failed to suppress hook")
		}
	}
}

func (c *Client) createWindow(ctx context.Context, sessionName, workDir string, w RenderedWindow) error {
	args := []string{"new-window", "-t", sessionName, "-n", w.Name}
	args = appendInitialPaneArgs(args, w, workDir)

	c.log.Debug().Strs("args", args).Msg("tmux new-window")
	if out, err := c.exec.Run(ctx, "tmux", args...); err != nil {
		return fmt.Errorf("tmux new-window %q: %w; output: %s", w.Name, err, strings.TrimSpace(string(out)))
	}
	c.tagPanesWithSession(ctx, sessionName+":"+w.Name, sessionName)
	return c.splitAdditionalPanes(ctx, sessionName, workDir, w)
}

func (c *Client) splitAdditionalPanes(ctx context.Context, sessionName, workDir string, w RenderedWindow) error {
	windowTarget := sessionName + ":" + w.Name
	for _, pane := range additionalPanes(w) {
		args := splitPaneArgs(windowTarget, pane, windowDir(w, workDir))
		c.log.Debug().Strs("args", args).Msg("tmux split-window")
		if out, err := c.exec.Run(ctx, "tmux", args...); err != nil {
			return fmt.Errorf("tmux split-window %q: %w; output: %s", w.Name, err, strings.TrimSpace(string(out)))
		}
		c.tagPanesWithSession(ctx, windowTarget, sessionName)
	}
	return nil
}

func appendInitialPaneArgs(args []string, w RenderedWindow, sessionDir string) []string {
	command := w.Command
	dir := windowDir(w, sessionDir)
	if len(w.Panes) > 0 {
		command = w.Panes[0].Command
		if w.Panes[0].Dir != "" {
			dir = w.Panes[0].Dir
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

func splitPaneArgs(target string, p RenderedPane, fallbackDir string) []string {
	args := []string{"split-window", "-t", target}
	if p.Split == "horizontal" {
		args = append(args, "-h")
	} else {
		args = append(args, "-v")
	}
	if p.Size != "" {
		args = append(args, "-l", p.Size)
	}
	dir := p.Dir
	if dir == "" {
		dir = fallbackDir
	}
	if dir != "" {
		args = append(args, "-c", dir)
	}
	if p.Command != "" {
		args = append(args, "--", "sh", "-c", p.Command)
	}
	return args
}

func additionalPanes(w RenderedWindow) []RenderedPane {
	if len(w.Panes) <= 1 {
		return nil
	}
	return w.Panes[1:]
}

// windowDir returns the working directory for a window, falling back to the session default.
func windowDir(w RenderedWindow, sessionDir string) string {
	if w.Dir != "" {
		return w.Dir
	}
	return sessionDir
}

// insideTmux reports whether the current process is running inside tmux.
var insideTmux = func() bool {
	return strings.TrimSpace(os.Getenv("TMUX")) != ""
}

// DetectCurrentTmuxSession returns the current tmux session name, or empty if not in tmux.
func DetectCurrentTmuxSession() string {
	cmd := exec.Command("tmux", "display-message", "-p", "#S")
	output, err := cmd.Output()
	if err != nil {
		log.Debug().Err(err).Msg("tmux session detection failed")
		return ""
	}
	return strings.TrimSpace(string(output))
}
