// Package tmux provides a Go-native tmux session client that creates sessions
// from declarative window definitions.
package tmux

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"

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

// ErrCommandExited matches a *CommandExitedError with errors.Is.
var ErrCommandExited = errors.New("tmux window command exited during startup")

// exitStatusNotFound is the status sh returns when it cannot find the command.
const exitStatusNotFound = 127

// CommandExitedError reports a window or pane command that exited while hive
// was still setting up the tmux session. The session is killed before the
// error is returned.
type CommandExitedError struct {
	Session string
	Window  string
	Command string
	Status  int    // exit status; -1 when unknown (e.g. killed by a signal)
	Output  string // last lines the command printed; may be empty
}

func (e *CommandExitedError) Error() string {
	msg := fmt.Sprintf("tmux session %q: window %q: command %q exited during startup", e.Session, e.Window, e.Command)
	switch {
	case e.NotFound():
		msg += " (status 127: command not found)"
	case e.Status >= 0:
		msg += fmt.Sprintf(" (status %d)", e.Status)
	}
	if e.Output != "" {
		msg += "; output: " + e.Output
	}
	return msg
}

func (e *CommandExitedError) Is(target error) bool {
	return target == ErrCommandExited
}

// NotFound reports whether the shell could not find the command.
func (e *CommandExitedError) NotFound() bool {
	return e.Status == exitStatusNotFound
}

// defaultStartupGrace is how long hive watches new commands before it treats
// them as started. A missing binary makes sh exit within a few milliseconds,
// but tmux still reports the pane as alive right after respawn-pane returns.
const defaultStartupGrace = 250 * time.Millisecond

const startupPollInterval = 50 * time.Millisecond

// Client creates and manages tmux sessions from window definitions.
type Client struct {
	exec         executil.Executor
	log          zerolog.Logger
	startupGrace time.Duration
}

// New creates a Client with the given executor and logger.
func New(exec executil.Executor, log zerolog.Logger) *Client {
	return &Client{exec: exec, log: log, startupGrace: defaultStartupGrace}
}

// startedPane is a pane hive launched a command in and watches until the
// startup grace ends.
type startedPane struct {
	id      string
	window  string
	command string
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
	started, err := c.newPane(ctx, []string{"new-session", "-d", "-s", name, "-n", first.Name}, name, workDir, first)
	if err != nil {
		return err
	}

	// Tag the initial pane for hive-managed pane identification.
	c.tagPanesWithSession(ctx, name, name)

	// Suppress interactive hooks (e.g. after-new-window command-prompt) that
	// block the tmux server waiting for input that will never arrive when
	// windows are created programmatically.
	c.suppressInteractiveHooks(ctx, name)

	split, err := c.splitAdditionalPanes(ctx, name, workDir, first)
	started = append(started, split...)
	if err != nil {
		c.killSession(ctx, name)
		return err
	}

	// Create additional windows. On failure, kill the partial session.
	for _, w := range windows[1:] {
		panes, err := c.createWindow(ctx, name, workDir, w)
		started = append(started, panes...)
		if err != nil {
			c.killSession(ctx, name)
			return err
		}
	}

	if err := c.awaitStartup(ctx, name, started); err != nil {
		c.killSession(ctx, name)
		return err
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

// killSession removes a partially created session. The "=" prefix makes tmux
// match the name exactly; a bare name also matches other sessions that start
// with it.
func (c *Client) killSession(ctx context.Context, name string) {
	if out, err := c.exec.Run(ctx, "tmux", "kill-session", "-t", "="+name); err != nil {
		c.log.Debug().Err(err).Str("session", name).Str("output", strings.TrimSpace(string(out))).Msg("failed to kill partial tmux session")
	}
}

// AddWindows adds windows to an existing tmux session.
// If any window has Focus set, that window is selected after all windows are created.
func (c *Client) AddWindows(ctx context.Context, name, workDir string, windows []RenderedWindow) error {
	c.suppressInteractiveHooks(ctx, name)
	var started []startedPane
	for _, w := range windows {
		panes, err := c.createWindow(ctx, name, workDir, w)
		started = append(started, panes...)
		if err != nil {
			return err
		}
	}
	if err := c.awaitStartup(ctx, name, started); err != nil {
		return err
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

func (c *Client) createWindow(ctx context.Context, sessionName, workDir string, w RenderedWindow) ([]startedPane, error) {
	started, err := c.newPane(ctx, []string{"new-window", "-t", sessionName, "-n", w.Name}, sessionName, workDir, w)
	if err != nil {
		return nil, err
	}
	c.tagPanesWithSession(ctx, sessionName+":"+w.Name, sessionName)
	split, err := c.splitAdditionalPanes(ctx, sessionName, workDir, w)
	return append(started, split...), err
}

// newPane runs baseArgs (new-session or new-window) to create the window's
// initial pane and starts its command.
//
// A window that runs any command gets remain-on-exit so a command that exits
// at once leaves a dead pane with its exit status and output, instead of
// closing the window (and the session, when it is the only window). The
// initial pane starts as a placeholder `cat` and the real command is started
// with respawn-pane after remain-on-exit is set; starting the command
// directly would let it exit before the option applies.
func (c *Client) newPane(ctx context.Context, baseArgs []string, sessionName, workDir string, w RenderedWindow) ([]startedPane, error) {
	command, dir := initialPane(w, workDir)
	op := baseArgs[0]

	args := slices.Concat(baseArgs, []string{"-P", "-F", "#{pane_id}"})
	if dir != "" {
		args = append(args, "-c", dir)
	}
	if command != "" {
		args = append(args, "--", "cat")
	}

	c.log.Debug().Strs("args", args).Msg("tmux " + op)
	out, err := c.exec.Run(ctx, "tmux", args...)
	if err != nil {
		if op == "new-window" {
			return nil, fmt.Errorf("tmux new-window %q: %w; output: %s", w.Name, err, strings.TrimSpace(string(out)))
		}
		return nil, fmt.Errorf("tmux %s: %w; output: %s", op, err, strings.TrimSpace(string(out)))
	}
	paneID := strings.TrimSpace(string(out))

	if !windowRunsCommand(w) {
		return nil, nil
	}

	target := sessionName + ":" + w.Name
	if paneID != "" {
		target = paneID
	}
	if out, err := c.exec.Run(ctx, "tmux", "set-option", "-w", "-t", target, "remain-on-exit", "on"); err != nil {
		return nil, fmt.Errorf("tmux set-option remain-on-exit %q: %w; output: %s", w.Name, err, strings.TrimSpace(string(out)))
	}
	if command == "" {
		return nil, nil
	}

	respawn := []string{"respawn-pane", "-k", "-t", target}
	if dir != "" {
		respawn = append(respawn, "-c", dir)
	}
	respawn = append(respawn, "--", "sh", "-c", command)
	c.log.Debug().Strs("args", respawn).Msg("tmux respawn-pane")
	if out, err := c.exec.Run(ctx, "tmux", respawn...); err != nil {
		return nil, fmt.Errorf("tmux respawn-pane %q: %w; output: %s", w.Name, err, strings.TrimSpace(string(out)))
	}
	return watchPane(paneID, w.Name, command), nil
}

func (c *Client) splitAdditionalPanes(ctx context.Context, sessionName, workDir string, w RenderedWindow) ([]startedPane, error) {
	windowTarget := sessionName + ":" + w.Name
	var started []startedPane
	for _, pane := range additionalPanes(w) {
		args := splitPaneArgs(windowTarget, pane, windowDir(w, workDir))
		c.log.Debug().Strs("args", args).Msg("tmux split-window")
		out, err := c.exec.Run(ctx, "tmux", args...)
		if err != nil {
			return started, fmt.Errorf("tmux split-window %q: %w; output: %s", w.Name, err, strings.TrimSpace(string(out)))
		}
		if pane.Command != "" {
			started = append(started, watchPane(strings.TrimSpace(string(out)), w.Name, pane.Command)...)
		}
		c.tagPanesWithSession(ctx, windowTarget, sessionName)
	}
	return started, nil
}

// watchPane returns the pane to watch, or nothing when tmux did not report
// its id and it cannot be identified later.
func watchPane(id, window, command string) []startedPane {
	if id == "" {
		return nil
	}
	return []startedPane{{id: id, window: window, command: command}}
}

// awaitStartup watches the started panes for the startup grace. When one has
// died it returns a *CommandExitedError; otherwise it clears remain-on-exit so
// commands that exit later close their window as usual.
func (c *Client) awaitStartup(ctx context.Context, sessionName string, started []startedPane) error {
	if len(started) == 0 {
		return nil
	}

	deadline := time.Now().Add(c.startupGrace)
	for {
		exited, err := c.findExitedPane(ctx, sessionName, started)
		if err != nil {
			return err
		}
		if exited != nil {
			return exited
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(min(remaining, startupPollInterval)):
		}
	}

	for _, p := range started {
		if out, err := c.exec.Run(ctx, "tmux", "set-option", "-w", "-u", "-t", p.id, "remain-on-exit"); err != nil {
			c.log.Warn().Err(err).Str("pane", p.id).Str("output", strings.TrimSpace(string(out))).Msg("failed to clear remain-on-exit")
		}
	}
	return nil
}

func (c *Client) findExitedPane(ctx context.Context, sessionName string, started []startedPane) (*CommandExitedError, error) {
	out, err := c.exec.Run(ctx, "tmux", "list-panes", "-s", "-t", sessionName, "-F", "#{pane_id} #{pane_dead} #{pane_dead_status}")
	if err != nil {
		return nil, fmt.Errorf("tmux list-panes: %w; output: %s", err, strings.TrimSpace(string(out)))
	}

	for line := range strings.Lines(string(out)) {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[1] != "1" {
			continue
		}
		for _, p := range started {
			if p.id != fields[0] {
				continue
			}
			status := -1
			if len(fields) > 2 {
				if n, err := strconv.Atoi(fields[2]); err == nil {
					status = n
				}
			}
			return &CommandExitedError{
				Session: sessionName,
				Window:  p.window,
				Command: p.command,
				Status:  status,
				Output:  c.deadPaneOutput(ctx, p.id),
			}, nil
		}
	}
	return nil, nil
}

// deadPaneOutput returns the last lines a dead pane printed, dropping the
// "Pane is dead" banner tmux adds. It is best-effort.
func (c *Client) deadPaneOutput(ctx context.Context, paneID string) string {
	out, err := c.exec.Run(ctx, "tmux", "capture-pane", "-p", "-J", "-t", paneID, "-S", "-20")
	if err != nil {
		c.log.Debug().Err(err).Str("pane", paneID).Msg("failed to capture dead pane output")
		return ""
	}
	var lines []string
	for line := range strings.Lines(string(out)) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Pane is dead") {
			continue
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "; ")
}

// initialPane returns the command and working directory of a window's first pane.
func initialPane(w RenderedWindow, sessionDir string) (command, dir string) {
	command = w.Command
	dir = windowDir(w, sessionDir)
	if len(w.Panes) > 0 {
		command = w.Panes[0].Command
		if w.Panes[0].Dir != "" {
			dir = w.Panes[0].Dir
		}
	}
	return command, dir
}

func windowRunsCommand(w RenderedWindow) bool {
	if command, _ := initialPane(w, ""); command != "" {
		return true
	}
	for _, p := range additionalPanes(w) {
		if p.Command != "" {
			return true
		}
	}
	return false
}

func splitPaneArgs(target string, p RenderedPane, fallbackDir string) []string {
	args := []string{"split-window", "-t", target, "-P", "-F", "#{pane_id}"}
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
