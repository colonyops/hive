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

// SpawnData is the template context for spawn commands.
type SpawnData struct {
	Path       string // Absolute path to session directory
	Name       string // Session name (display name)
	Prompt     string // User-provided prompt (batch only)
	Slug       string // Session slug (URL-safe version of name)
	ContextDir string // Path to context directory
	Owner      string // Repository owner
	Repo       string // Repository name
}

// Spawner handles terminal spawning with template rendering.
type Spawner struct {
	log      zerolog.Logger
	executor executil.Executor
	renderer *tmpl.Renderer
	tmux     *coretmux.Client
	stdout   io.Writer
	stderr   io.Writer
}

// NewSpawner creates a new Spawner.
func NewSpawner(log zerolog.Logger, executor executil.Executor, renderer *tmpl.Renderer, stdout, stderr io.Writer) *Spawner {
	return &Spawner{
		log:      log,
		executor: executor,
		renderer: renderer,
		tmux:     coretmux.New(executor),
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

// renderWindow renders a single WindowConfig's template fields against SpawnData.
func renderWindow(renderer *tmpl.Renderer, w config.WindowConfig, data SpawnData) (coretmux.RenderedWindow, error) {
	name, err := renderer.Render(w.Name, data)
	if err != nil {
		return coretmux.RenderedWindow{}, fmt.Errorf("name template: %w", err)
	}

	var command string
	if w.Command != "" {
		command, err = renderer.Render(w.Command, data)
		if err != nil {
			return coretmux.RenderedWindow{}, fmt.Errorf("command template: %w", err)
		}
		command = strings.TrimSpace(command)
	}

	var dir string
	if w.Dir != "" {
		dir, err = renderer.Render(w.Dir, data)
		if err != nil {
			return coretmux.RenderedWindow{}, fmt.Errorf("dir template: %w", err)
		}
	}

	return coretmux.RenderedWindow{
		Name:    name,
		Command: command,
		Dir:     dir,
		Focus:   w.Focus,
	}, nil
}
