package pipeline

import (
	"context"
	"fmt"
	"strings"

	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/desktop/pipeline/actions"
	"github.com/colonyops/hive/pkg/tmpl"
)

// LaunchSessionRequest is a rendered launch-session action, ready to hand to
// a SessionLauncher.
type LaunchSessionRequest struct {
	Name   string
	Prompt string
	Agent  string
	Repo   string
}

// SessionLauncher spawns a hive session for a launch-session action.
type SessionLauncher interface {
	LaunchSession(ctx context.Context, req LaunchSessionRequest) error
}

// LaunchSessionExecutor renders a launch-session action's templates over
// the triggering msg and hands the result to a SessionLauncher.
type LaunchSessionExecutor struct {
	launcher SessionLauncher
}

// NewLaunchSessionExecutor builds a LaunchSessionExecutor over launcher.
// A nil launcher leaves the executor unavailable rather than acknowledging an
// action without creating its session.
func NewLaunchSessionExecutor(launcher SessionLauncher) *LaunchSessionExecutor {
	return &LaunchSessionExecutor{launcher: launcher}
}

func (e *LaunchSessionExecutor) Execute(ctx context.Context, action actions.Action, data OutputData) error {
	cfg, ok := action.Config.(*actions.LaunchSessionConfig)
	if !ok {
		return fmt.Errorf("launch-session executor: action %q has config type %T", action.ID, action.Config)
	}
	if e.launcher == nil {
		return fmt.Errorf("launch-session executor: no session launcher configured")
	}

	renderer := tmpl.New(tmpl.Config{})

	prompt, err := renderer.Render(cfg.PromptTemplate, data)
	if err != nil {
		return fmt.Errorf("launch-session: prompt_template: %w", err)
	}
	prompt = strings.TrimSpace(prompt)

	var repo string
	if cfg.RepoTemplate != "" {
		repo, err = renderer.Render(cfg.RepoTemplate, data)
		if err != nil {
			return fmt.Errorf("launch-session: repo_template: %w", err)
		}
		repo = strings.TrimSpace(repo)
	}

	name := session.Slugify(action.ID + "-" + data.Key)
	if err := session.ValidateName(name); err != nil {
		return fmt.Errorf("launch-session: derived session name: %w", err)
	}

	return e.launcher.LaunchSession(ctx, LaunchSessionRequest{
		Name:   name,
		Prompt: prompt,
		Agent:  cfg.Agent,
		Repo:   repo,
	})
}
