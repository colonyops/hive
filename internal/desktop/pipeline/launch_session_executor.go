package pipeline

import (
	"context"
	"fmt"
	"strings"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/desktop/pipeline/actions"
	"github.com/colonyops/hive/pkg/tmpl"
)

// LaunchSessionRequest is a rendered launch-session action, ready to hand to
// a SessionLauncher.
type LaunchSessionRequest struct {
	Prompt string
	Agent  string
	Repo   string
	Post   string
}

// SessionLauncher spawns a real hive session for a launch-session action.
//
// The desktop app does not wire the hive session service yet (see
// internal/desktop/desktop.go's package doc: session/core logic is
// deliberately excluded from the desktop build), so there is no real
// implementation of this interface in this phase. LoggingSessionLauncher is
// the default: it logs the rendered request and returns success without
// spawning anything. Real session-spawn wiring — the flagship
// "launch-session" scenario the design calls out (an incoming PR auto-spawns
// a review agent) — is deferred to a later phase, which will provide a real
// SessionLauncher (almost certainly backed by internal/hive.SessionService)
// and inject it here; LaunchSessionExecutor does not need to change.
type SessionLauncher interface {
	LaunchSession(ctx context.Context, req LaunchSessionRequest) error
}

// LoggingSessionLauncher is the default SessionLauncher: a stub that logs
// what it would have spawned. See SessionLauncher's doc for why this is the
// default rather than a real implementation.
type LoggingSessionLauncher struct {
	Logger zerolog.Logger
}

func (l LoggingSessionLauncher) LaunchSession(_ context.Context, req LaunchSessionRequest) error {
	l.Logger.Info().
		Str("agent", req.Agent).
		Str("repo", req.Repo).
		Str("post", req.Post).
		Str("prompt", req.Prompt).
		Msg("launch-session action (stub): would spawn a hive session")
	return nil
}

// LaunchSessionExecutor renders a launch-session action's templates over
// the triggering msg and hands the result to a SessionLauncher.
type LaunchSessionExecutor struct {
	launcher SessionLauncher
}

// NewLaunchSessionExecutor builds a LaunchSessionExecutor over launcher. A
// nil launcher defaults to LoggingSessionLauncher.
func NewLaunchSessionExecutor(launcher SessionLauncher) *LaunchSessionExecutor {
	if launcher == nil {
		launcher = LoggingSessionLauncher{Logger: zerolog.Nop()}
	}
	return &LaunchSessionExecutor{launcher: launcher}
}

func (e *LaunchSessionExecutor) Execute(ctx context.Context, action actions.Action, data OutputData) error {
	cfg, ok := action.Config.(*actions.LaunchSessionConfig)
	if !ok {
		return fmt.Errorf("launch-session executor: action %q has config type %T", action.ID, action.Config)
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
	}

	return e.launcher.LaunchSession(ctx, LaunchSessionRequest{
		Prompt: prompt,
		Agent:  cfg.Agent,
		Repo:   repo,
		Post:   cfg.Post,
	})
}
