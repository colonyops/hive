package command

import (
	"context"
	"fmt"
	"io"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/git"
	"github.com/colonyops/hive/internal/core/session"
	coretmux "github.com/colonyops/hive/internal/core/tmux"
	"github.com/colonyops/hive/pkg/tmpl"
)

// SessionDeleter is the interface for deleting sessions.
type SessionDeleter interface {
	DeleteSession(ctx context.Context, id string) error
}

// SessionRecycler is the interface for recycling sessions.
type SessionRecycler interface {
	RecycleSession(ctx context.Context, id string, w io.Writer) error
}

// Service creates command executors based on action type.
type Service struct {
	deleter  SessionDeleter
	recycler SessionRecycler
	cfg      *config.Config
	renderer *tmpl.Renderer
	tmux     *coretmux.Builder
}

// NewService creates a new command service with the given dependencies.
func NewService(deleter SessionDeleter, recycler SessionRecycler, cfg *config.Config, renderer *tmpl.Renderer, tmux *coretmux.Builder) *Service {
	return &Service{
		deleter:  deleter,
		recycler: recycler,
		cfg:      cfg,
		renderer: renderer,
		tmux:     tmux,
	}
}

// ActionType identifies the kind of action.
type ActionType int

const (
	ActionTypeNone ActionType = iota
	ActionTypeRecycle
	ActionTypeDelete
	ActionTypeShell
	ActionTypeTmuxOpen
	ActionTypeTmuxStart
)

// Action represents a resolved command action ready for execution.
type Action struct {
	Type          ActionType
	Key           string
	Help          string
	Confirm       string // Non-empty if confirmation required
	ShellCmd      string // For shell actions, the rendered command
	SessionID     string
	SessionName   string // Session display name (for tmux actions)
	SessionPath   string
	SessionRemote string // Session remote URL (for tmux actions)
	Silent        bool   // Skip loading popup for fast commands
	Exit          bool   // Exit hive after command completes
	Err           error  // Non-nil if action resolution failed (e.g., template error)
}

// NeedsConfirm returns true if the action requires user confirmation.
func (a Action) NeedsConfirm() bool {
	return a.Confirm != ""
}

// CreateExecutor creates an executor for the given action.
// Returns error if the action type is not supported.
func (s *Service) CreateExecutor(action Action) (Executor, error) {
	switch action.Type {
	case ActionTypeDelete:
		return &DeleteExecutor{
			deleter:   s.deleter,
			sessionID: action.SessionID,
		}, nil
	case ActionTypeRecycle:
		return &RecycleExecutor{
			recycler:  s.recycler,
			sessionID: action.SessionID,
		}, nil
	case ActionTypeShell:
		return &ShellExecutor{
			cmd: action.ShellCmd,
		}, nil
	case ActionTypeTmuxOpen:
		return s.newTmuxExecutor(action, false)
	case ActionTypeTmuxStart:
		return s.newTmuxExecutor(action, true)
	default:
		return nil, fmt.Errorf("unsupported action type: %d", action.Type)
	}
}

func (s *Service) newTmuxExecutor(action Action, background bool) (Executor, error) {
	strategy := s.cfg.ResolveSpawn(action.SessionRemote, false)
	if !strategy.IsWindows() {
		// Legacy: fall back to shell executor with hive-tmux script
		return nil, fmt.Errorf("tmux action requires windows config (legacy spawn commands should use shell executor)")
	}

	// Render window templates with full spawn data
	windows := make([]coretmux.RenderedWindow, 0, len(strategy.Windows))
	owner, repo := git.ExtractOwnerRepo(action.SessionRemote)
	data := struct {
		Path       string
		Name       string
		Prompt     string
		Slug       string
		ContextDir string
		Owner      string
		Repo       string
	}{
		Path:       action.SessionPath,
		Name:       action.SessionName,
		Slug:       session.Slugify(action.SessionName),
		ContextDir: s.cfg.RepoContextDir(owner, repo),
		Owner:      owner,
		Repo:       repo,
	}

	for _, w := range strategy.Windows {
		name, err := s.renderer.Render(w.Name, data)
		if err != nil {
			return nil, fmt.Errorf("render window name %q: %w", w.Name, err)
		}

		var cmd string
		if w.Command != "" {
			cmd, err = s.renderer.Render(w.Command, data)
			if err != nil {
				return nil, fmt.Errorf("render window command: %w", err)
			}
		}

		var dir string
		if w.Dir != "" {
			dir, err = s.renderer.Render(w.Dir, data)
			if err != nil {
				return nil, fmt.Errorf("render window dir: %w", err)
			}
		}

		windows = append(windows, coretmux.RenderedWindow{
			Name:    name,
			Command: cmd,
			Dir:     dir,
			Focus:   w.Focus,
		})
	}

	return &TmuxExecutor{
		builder:    s.tmux,
		name:       action.SessionName,
		workDir:    action.SessionPath,
		windows:    windows,
		background: background,
	}, nil
}
