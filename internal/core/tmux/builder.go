// Package tmux provides a Go-native tmux session builder that creates sessions
// from declarative window definitions.
package tmux

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/colonyops/hive/pkg/executil"
)

// RenderedWindow is a fully-resolved tmux window definition (no templates).
type RenderedWindow struct {
	Name    string // Window name
	Command string // Command to run (empty = default shell)
	Dir     string // Working directory (empty = session default)
	Focus   bool   // Select this window after creation
}

// Builder creates and manages tmux sessions from window definitions.
type Builder struct {
	exec executil.Executor
}

// NewBuilder creates a Builder with the given executor.
func NewBuilder(exec executil.Executor) *Builder {
	return &Builder{exec: exec}
}

// HasSession checks whether a tmux session with the given name exists.
func (b *Builder) HasSession(ctx context.Context, name string) bool {
	_, err := b.exec.Run(ctx, "tmux", "has-session", "-t", name)
	return err == nil
}

// CreateSession creates a tmux session with the given windows.
// The first window is created via new-session; additional windows via new-window.
// If background is true, the session is created detached.
func (b *Builder) CreateSession(ctx context.Context, name, workDir string, windows []RenderedWindow, background bool) error {
	if len(windows) == 0 {
		return fmt.Errorf("tmux: at least one window is required")
	}

	// Create session with the first window.
	first := windows[0]
	args := []string{"new-session", "-d", "-s", name, "-n", first.Name}
	if dir := windowDir(first, workDir); dir != "" {
		args = append(args, "-c", dir)
	}
	if first.Command != "" {
		args = append(args, "--", "sh", "-c", first.Command)
	}

	if _, err := b.exec.Run(ctx, "tmux", args...); err != nil {
		return fmt.Errorf("tmux new-session: %w", err)
	}

	// Create additional windows.
	for _, w := range windows[1:] {
		args := []string{"new-window", "-t", name, "-n", w.Name}
		if dir := windowDir(w, workDir); dir != "" {
			args = append(args, "-c", dir)
		}
		if w.Command != "" {
			args = append(args, "--", "sh", "-c", w.Command)
		}

		if _, err := b.exec.Run(ctx, "tmux", args...); err != nil {
			return fmt.Errorf("tmux new-window %q: %w", w.Name, err)
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
	if _, err := b.exec.Run(ctx, "tmux", "select-window", "-t", name+":"+focusName); err != nil {
		return fmt.Errorf("tmux select-window: %w", err)
	}

	if !background {
		return b.AttachOrSwitch(ctx, name)
	}
	return nil
}

// AttachOrSwitch connects to an existing tmux session.
// Inside tmux it uses switch-client; outside it uses attach-session.
func (b *Builder) AttachOrSwitch(ctx context.Context, name string) error {
	if insideTmux() {
		_, err := b.exec.Run(ctx, "tmux", "switch-client", "-t", name)
		if err != nil {
			return fmt.Errorf("tmux switch-client: %w", err)
		}
		return nil
	}

	_, err := b.exec.Run(ctx, "tmux", "attach-session", "-t", name)
	if err != nil {
		return fmt.Errorf("tmux attach-session: %w", err)
	}
	return nil
}

// OpenSession creates a session if it doesn't exist, or attaches to it.
// If targetWindow is non-empty and the session already exists, select that window before attaching.
func (b *Builder) OpenSession(ctx context.Context, name, workDir string, windows []RenderedWindow, background bool, targetWindow string) error {
	if b.HasSession(ctx, name) {
		if background {
			return nil
		}
		if targetWindow != "" {
			// Best-effort: window may not exist if config changed since session was created.
			_, _ = b.exec.Run(ctx, "tmux", "select-window", "-t", name+":"+targetWindow)
		}
		return b.AttachOrSwitch(ctx, name)
	}
	return b.CreateSession(ctx, name, workDir, windows, background)
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
