// Package hive provides the service layer for orchestrating hive operations.
package hive

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/colonyops/hive/internal/core/config"
	coretmux "github.com/colonyops/hive/internal/core/tmux"
	"github.com/colonyops/hive/pkg/executil"
	"github.com/colonyops/hive/pkg/tmpl"
	"github.com/rs/zerolog"
)

// SessionClient is the interface used by consumers that create/open tmux sessions.
type SessionClient interface {
	CreateSession(ctx context.Context, name, workDir string, windows []coretmux.RenderedWindow, background bool) error
	OpenSession(ctx context.Context, name, workDir string, windows []coretmux.RenderedWindow, background bool, targetWindow string) error
	AddWindows(ctx context.Context, name, workDir string, windows []coretmux.RenderedWindow) error
	AttachOrSwitch(ctx context.Context, name string) error
}

// SpawnData is the template context for spawn commands.
type SpawnData struct {
	Path       string         // Absolute path to session directory
	Name       string         // Session name (display name)
	Prompt     string         // User-provided prompt (batch only)
	Slug       string         // Session slug (URL-safe version of name)
	ContextDir string         // Path to context directory
	Owner      string         // Repository owner
	Repo       string         // Repository name
	Vars       map[string]any // User-defined template variables
}

// Spawner handles terminal spawning with template rendering.
type Spawner struct {
	log      zerolog.Logger
	executor executil.Executor
	renderer *tmpl.Renderer
	tmux     SessionClient
	stdout   io.Writer
	stderr   io.Writer
}

// NewSpawner creates a new Spawner.
func NewSpawner(log zerolog.Logger, executor executil.Executor, renderer *tmpl.Renderer, tmuxClient SessionClient, stdout, stderr io.Writer) *Spawner {
	return &Spawner{
		log:      log,
		executor: executor,
		renderer: renderer,
		tmux:     tmuxClient,
		stdout:   stdout,
		stderr:   stderr,
	}
}

// Spawn executes spawn commands sequentially with template rendering.
func (s *Spawner) Spawn(ctx context.Context, commands []string, data SpawnData) error {
	for _, cmdTmpl := range commands {
		s.log.Debug().Str("command", cmdTmpl).Msg("executing spawn command")

		rendered, err := s.renderer.Render(cmdTmpl, data)
		if err != nil {
			return fmt.Errorf("render spawn command %q: %w", cmdTmpl, err)
		}

		if err := s.executor.RunStream(ctx, s.stdout, s.stderr, "sh", "-c", rendered); err != nil {
			return fmt.Errorf("execute spawn command %q: %w", rendered, err)
		}
	}

	s.log.Debug().Msg("spawn complete")
	return nil
}

// SpawnWindows renders window templates and creates a tmux session.
func (s *Spawner) SpawnWindows(ctx context.Context, windows []config.WindowConfig, data SpawnData, background bool) error {
	rendered, err := RenderWindows(s.renderer, windows, data)
	if err != nil {
		return err
	}

	s.log.Debug().Int("windows", len(rendered)).Bool("background", background).Msg("spawning tmux session")

	if err := s.tmux.CreateSession(ctx, data.Name, data.Path, rendered, background); err != nil {
		return fmt.Errorf("create tmux session: %w", err)
	}

	s.log.Debug().Msg("spawn windows complete")
	return nil
}

// OpenWindows renders window templates and opens (or creates) a tmux session.
// If the session already exists, it attaches to it (optionally selecting targetWindow).
func (s *Spawner) OpenWindows(ctx context.Context, windows []config.WindowConfig, data SpawnData, background bool, targetWindow string) error {
	rendered, err := RenderWindows(s.renderer, windows, data)
	if err != nil {
		return err
	}

	s.log.Debug().Int("windows", len(rendered)).Bool("background", background).Str("targetWindow", targetWindow).Msg("opening tmux session")

	if err := s.tmux.OpenSession(ctx, data.Name, data.Path, rendered, background, targetWindow); err != nil {
		return fmt.Errorf("open tmux session: %w", err)
	}

	return nil
}

// RenderWindows renders a slice of WindowConfig templates against SpawnData,
// producing fully-resolved RenderedWindow values ready for the tmux Client.
func RenderWindows(renderer *tmpl.Renderer, windows []config.WindowConfig, data SpawnData) ([]coretmux.RenderedWindow, error) {
	rendered := make([]coretmux.RenderedWindow, 0, len(windows))
	for _, w := range windows {
		rw, err := renderWindow(renderer, w, data)
		if err != nil {
			return nil, fmt.Errorf("render window %q: %w", w.Name, err)
		}
		rendered = append(rendered, rw)
	}
	return rendered, nil
}

// renderWindowCommon is the shared rendering core used by renderWindow and renderWindowMap.
// render is a closure that evaluates a single template string against the caller's data context.
func renderWindowCommon(w config.WindowConfig, render func(string) (string, error)) (coretmux.RenderedWindow, error) {
	name, err := render(w.Name)
	if err != nil {
		return coretmux.RenderedWindow{}, fmt.Errorf("name template: %w", err)
	}

	var command string
	if w.Command != "" {
		command, err = render(w.Command)
		if err != nil {
			return coretmux.RenderedWindow{}, fmt.Errorf("command template: %w", err)
		}
		command = strings.TrimSpace(command)
	}

	var dir string
	if w.Dir != "" {
		dir, err = render(w.Dir)
		if err != nil {
			return coretmux.RenderedWindow{}, fmt.Errorf("dir template: %w", err)
		}
	}

	return coretmux.RenderedWindow{Name: name, Command: command, Dir: dir, Focus: w.Focus}, nil
}

// renderWindow renders a single WindowConfig against SpawnData.
func renderWindow(renderer *tmpl.Renderer, w config.WindowConfig, data SpawnData) (coretmux.RenderedWindow, error) {
	return renderWindowCommon(w, func(tmplStr string) (string, error) {
		return renderer.Render(tmplStr, data)
	})
}

// renderWindowMap renders a single WindowConfig against a map[string]any data context.
// Used for UserCommand windows, which carry .Form and session variables as a map.
func renderWindowMap(renderer *tmpl.Renderer, w config.WindowConfig, data map[string]any) (coretmux.RenderedWindow, error) {
	return renderWindowCommon(w, func(tmplStr string) (string, error) {
		return renderer.Render(tmplStr, data)
	})
}

// RenderUserCommandWindows renders windows from a UserCommand using the provided template data map.
// Unlike RenderWindows, it accepts map[string]any to include .Form and session variables.
func RenderUserCommandWindows(renderer *tmpl.Renderer, windows []config.WindowConfig, data map[string]any) ([]coretmux.RenderedWindow, error) {
	rendered := make([]coretmux.RenderedWindow, 0, len(windows))
	for _, w := range windows {
		rw, err := renderWindowMap(renderer, w, data)
		if err != nil {
			return nil, fmt.Errorf("render window %q: %w", w.Name, err)
		}
		rendered = append(rendered, rw)
	}
	return rendered, nil
}

// AddWindowsToTmuxSession adds pre-rendered windows to an existing tmux session.
// If background is false, switches to the session after adding windows.
func (s *Spawner) AddWindowsToTmuxSession(ctx context.Context, tmuxName, workDir string, windows []coretmux.RenderedWindow, background bool) error {
	if err := s.tmux.AddWindows(ctx, tmuxName, workDir, windows); err != nil {
		return fmt.Errorf("add windows to %q: %w", tmuxName, err)
	}
	if !background {
		if err := s.tmux.AttachOrSwitch(ctx, tmuxName); err != nil {
			return fmt.Errorf("switch to session %q: %w", tmuxName, err)
		}
	}
	return nil
}
