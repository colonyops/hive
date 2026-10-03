// Package hive provides the service layer for orchestrating hive operations.
package hive

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/multiplexer"
	"github.com/colonyops/hive/pkg/executil"
	"github.com/colonyops/hive/pkg/tmpl"
	"github.com/rs/zerolog"
)

// SessionCreator is the interface used by consumers that create and open sessions.
type SessionCreator interface {
	CreateSession(ctx context.Context, spec multiplexer.SessionSpec) error
	OpenSession(ctx context.Context, spec multiplexer.SessionSpec, selection multiplexer.Target) error
	AddWindows(ctx context.Context, target multiplexer.Target, windows []multiplexer.WindowSpec) error
	AttachOrSwitch(ctx context.Context, target multiplexer.Target, streams multiplexer.AttachStreams) error
}

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
	tmux     SessionCreator
	stdout   io.Writer
	stderr   io.Writer
}

// NewSpawner creates a new Spawner.
func NewSpawner(log zerolog.Logger, executor executil.Executor, renderer *tmpl.Renderer, tmuxClient SessionCreator, stdout, stderr io.Writer) *Spawner {
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
	return s.SpawnWith(ctx, commands, data, s.renderer)
}

// SpawnWith executes spawn commands using the given renderer instead of the default.
func (s *Spawner) SpawnWith(ctx context.Context, commands []string, data SpawnData, renderer *tmpl.Renderer) error {
	for _, cmdTmpl := range commands {
		s.log.Debug().Str("command", cmdTmpl).Msg("executing spawn command")

		rendered, err := renderer.Render(cmdTmpl, data)
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
	return s.SpawnWindowsWith(ctx, windows, data, background, s.renderer)
}

// SpawnWindowsWith renders window templates using the given renderer and creates a tmux session.
func (s *Spawner) SpawnWindowsWith(ctx context.Context, windows []config.WindowConfig, data SpawnData, background bool, renderer *tmpl.Renderer) error {
	rendered, err := RenderWindows(renderer, windows, data)
	if err != nil {
		return err
	}

	s.log.Debug().Int("windows", len(rendered)).Bool("background", background).Msg("spawning tmux session")

	if err := s.tmux.CreateSession(ctx, multiplexer.SessionSpec{
		Target:           multiplexer.Target{Session: data.Slug},
		WorkingDirectory: data.Path,
		Windows:          rendered,
		Background:       background,
	}); err != nil {
		return fmt.Errorf("create tmux session: %w", err)
	}

	s.log.Debug().Msg("spawn windows complete")
	return nil
}

// OpenWindows renders window templates and opens (or creates) a tmux session.
// If the session already exists, it attaches to it (optionally selecting targetWindow).
func (s *Spawner) OpenWindows(ctx context.Context, windows []config.WindowConfig, data SpawnData, background bool, targetWindow string) error {
	return s.OpenWindowsWith(ctx, windows, data, background, targetWindow, s.renderer)
}

// OpenWindowsWith renders window templates using the given renderer and opens (or creates) a tmux session.
func (s *Spawner) OpenWindowsWith(ctx context.Context, windows []config.WindowConfig, data SpawnData, background bool, targetWindow string, renderer *tmpl.Renderer) error {
	rendered, err := RenderWindows(renderer, windows, data)
	if err != nil {
		return err
	}

	s.log.Debug().Int("windows", len(rendered)).Bool("background", background).Str("targetWindow", targetWindow).Msg("opening tmux session")

	if err := s.tmux.OpenSession(ctx, multiplexer.SessionSpec{
		Target:           multiplexer.Target{Session: data.Slug},
		WorkingDirectory: data.Path,
		Windows:          rendered,
		Background:       background,
	}, targetFromLegacy(data.Slug, targetWindow)); err != nil {
		return fmt.Errorf("open tmux session: %w", err)
	}

	return nil
}

// RenderWindows renders a slice of WindowConfig templates against SpawnData,
// producing fully resolved window specifications for the multiplexer client.
func RenderWindows(renderer *tmpl.Renderer, windows []config.WindowConfig, data SpawnData) ([]multiplexer.WindowSpec, error) {
	rendered := make([]multiplexer.WindowSpec, 0, len(windows))
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
func renderWindowCommon(w config.WindowConfig, render func(string) (string, error)) (multiplexer.WindowSpec, error) {
	name, err := render(w.Name)
	if err != nil {
		return multiplexer.WindowSpec{}, fmt.Errorf("name template: %w", err)
	}

	var command string
	if w.Command != "" {
		command, err = render(w.Command)
		if err != nil {
			return multiplexer.WindowSpec{}, fmt.Errorf("command template: %w", err)
		}
		command = strings.TrimSpace(command)
	}

	var dir string
	if w.Dir != "" {
		dir, err = render(w.Dir)
		if err != nil {
			return multiplexer.WindowSpec{}, fmt.Errorf("dir template: %w", err)
		}
	}

	panes, err := renderPanes(w.Panes, render)
	if err != nil {
		return multiplexer.WindowSpec{}, err
	}

	return multiplexer.WindowSpec{Name: name, Command: command, WorkingDirectory: dir, Focus: w.Focus, Panes: panes}, nil
}

func renderPanes(panes []config.PaneConfig, render func(string) (string, error)) ([]multiplexer.PaneSpec, error) {
	rendered := make([]multiplexer.PaneSpec, 0, len(panes))
	for i, p := range panes {
		rp, err := renderPane(p, render)
		if err != nil {
			return nil, fmt.Errorf("panes[%d]: %w", i, err)
		}
		rendered = append(rendered, rp)
	}
	return rendered, nil
}

func renderPane(p config.PaneConfig, render func(string) (string, error)) (multiplexer.PaneSpec, error) {
	var command string
	if p.Command != "" {
		rendered, err := render(p.Command)
		if err != nil {
			return multiplexer.PaneSpec{}, fmt.Errorf("command template: %w", err)
		}
		command = strings.TrimSpace(rendered)
	}

	var dir string
	if p.Dir != "" {
		rendered, err := render(p.Dir)
		if err != nil {
			return multiplexer.PaneSpec{}, fmt.Errorf("dir template: %w", err)
		}
		dir = rendered
	}

	var size string
	if p.Size != "" {
		rendered, err := render(p.Size)
		if err != nil {
			return multiplexer.PaneSpec{}, fmt.Errorf("size template: %w", err)
		}
		size = strings.TrimSpace(rendered)
	}

	return multiplexer.PaneSpec{Command: command, WorkingDirectory: dir, Size: size, Split: multiplexer.SplitDirection(p.Split)}, nil
}

// renderWindow renders a single WindowConfig against SpawnData.
func renderWindow(renderer *tmpl.Renderer, w config.WindowConfig, data SpawnData) (multiplexer.WindowSpec, error) {
	return renderWindowCommon(w, func(tmplStr string) (string, error) {
		return renderer.Render(tmplStr, data)
	})
}

// renderWindowMap renders a single WindowConfig against a map[string]any data context.
// Used for UserCommand windows, which carry .Form and session variables as a map.
func renderWindowMap(renderer *tmpl.Renderer, w config.WindowConfig, data map[string]any) (multiplexer.WindowSpec, error) {
	return renderWindowCommon(w, func(tmplStr string) (string, error) {
		return renderer.Render(tmplStr, data)
	})
}

// RenderUserCommandWindows renders windows from a UserCommand using the provided template data map.
// Unlike RenderWindows, it accepts map[string]any to include .Form and session variables.
func RenderUserCommandWindows(renderer *tmpl.Renderer, windows []config.WindowConfig, data map[string]any) ([]multiplexer.WindowSpec, error) {
	rendered := make([]multiplexer.WindowSpec, 0, len(windows))
	for _, w := range windows {
		rw, err := renderWindowMap(renderer, w, data)
		if err != nil {
			return nil, fmt.Errorf("render window %q: %w", w.Name, err)
		}
		rendered = append(rendered, rw)
	}
	return rendered, nil
}

// targetFromLegacy converts the TmuxWindow template value into a Target. The
// legacy value is either a window name/index or a native %N pane ID; a pane ID
// is carried in Pane with an empty Window so the adapter can resolve its window.
func targetFromLegacy(sessionName, selection string) multiplexer.Target {
	target := multiplexer.Target{Session: sessionName}
	if strings.HasPrefix(selection, "%") {
		target.Pane = selection
	} else {
		target.Window = selection
	}
	return target
}

// AddWindowsToTmuxSession adds pre-rendered windows to an existing tmux session.
// Windows without a working directory inherit workDir. If background is false,
// it switches to the session after adding windows.
func (s *Spawner) AddWindowsToTmuxSession(ctx context.Context, tmuxName, workDir string, windows []multiplexer.WindowSpec, background bool) error {
	for i := range windows {
		if windows[i].WorkingDirectory == "" {
			windows[i].WorkingDirectory = workDir
		}
	}
	target := multiplexer.Target{Session: tmuxName}
	if err := s.tmux.AddWindows(ctx, target, windows); err != nil {
		return fmt.Errorf("add windows to %q: %w", tmuxName, err)
	}
	if !background {
		if err := s.tmux.AttachOrSwitch(ctx, target, multiplexer.AttachStreams{Stdin: os.Stdin, Stdout: s.stdout, Stderr: s.stderr}); err != nil {
			return fmt.Errorf("switch to session %q: %w", tmuxName, err)
		}
	}
	return nil
}
