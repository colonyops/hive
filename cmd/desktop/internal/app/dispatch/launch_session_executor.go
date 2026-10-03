package dispatch

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/colonyops/hive/pkg/tmpl"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/observe"
)

// defaultPostHookTimeout is generous for a hook that hands the checkout to
// something else. Anything slower says so in post_hook_timeout.
const defaultPostHookTimeout = time.Minute

// LaunchSessionRequest is a rendered launch-session action, ready to hand to
// a SessionLauncher.
type LaunchSessionRequest struct {
	Name   string
	Prompt string
	Agent  string
	Repo   string
	// UniqueName suffixes a generated name whose slug a session already
	// holds instead of failing.
	UniqueName bool
	// Origins are the inbox items the session is being created for. An empty
	// slice means the session has no inbox item behind it.
	Origins []models.ItemRef
}

// SessionLauncher spawns a hive session for a launch-session action.
type SessionLauncher interface {
	LaunchSession(ctx context.Context, req LaunchSessionRequest) (SessionExecutionOutcome, error)
}

type LaunchWorkspaceSessionRequest struct {
	Workspace string
	Name      string
	Prompt    string
	Origins   []models.ItemRef
}

type WorkspaceSessionLauncher interface {
	LaunchWorkspaceSession(ctx context.Context, req LaunchWorkspaceSessionRequest) (SessionExecutionOutcome, error)
}

// LaunchSessionExecutor renders a launch-session action's templates over the
// triggering message and routes it to the configured target launcher.
type LaunchSessionExecutor struct {
	logger            zerolog.Logger
	launcher          SessionLauncher
	workspaceLauncher WorkspaceSessionLauncher
	env               ExecEnvironment
}

func NewLaunchSessionExecutor(logger zerolog.Logger, launcher SessionLauncher, workspaceLauncher WorkspaceSessionLauncher, env ExecEnvironment) *LaunchSessionExecutor {
	return &LaunchSessionExecutor{logger: logger, launcher: launcher, workspaceLauncher: workspaceLauncher, env: env}
}

func (e *LaunchSessionExecutor) Execute(ctx context.Context, action actions.Action, data OutputData, input ActionInvocationInput) (ExecutionResult, error) {
	var cfg *actions.LaunchSessionConfig
	var nameTemplate string
	// Errors use the key the author wrote, which differs for a node.
	promptField, repoField := "prompt_template", "repo_template"
	switch c := action.Config.(type) {
	case *actions.LaunchSessionConfig:
		cfg = c
	case *LaunchNodeActionConfig:
		cfg, nameTemplate = &c.LaunchSessionConfig, c.NameTemplate
		promptField, repoField = "prompt", "repo"
	default:
		return ExecutionResult{}, fmt.Errorf("launch-session executor: action %q has config type %T", action.ID, action.Config)
	}

	prompt, err := tmpl.New(tmpl.Config{}).Render(cfg.PromptTemplate, data)
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("launch-session: %s: %w", promptField, err)
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return ExecutionResult{}, fmt.Errorf("launch-session: %s rendered blank", promptField)
	}

	name, err := sessionName(action.ID, nameTemplate, data)
	if err != nil {
		return ExecutionResult{}, err
	}

	repo, err := renderRepoTemplate(cfg.RepoTemplate, data.Key, data.Raw, data.Inputs)
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("launch-session: %s: %w", repoField, err)
	}
	workspace := strings.TrimSpace(cfg.Workspace)
	agent := cfg.Agent
	generatedName := true
	if strings.TrimSpace(cfg.RepoTemplate) == "" && workspace == "" {
		if input.Session == nil {
			return ExecutionResult{}, fmt.Errorf("launch-session: target and session name input are required")
		}
		repo = strings.TrimSpace(input.Session.Repository)
		workspace = strings.TrimSpace(input.Session.Workspace)
		name = strings.TrimSpace(input.Session.Name)
		generatedName = false
		if (repo == "") == (workspace == "") {
			return ExecutionResult{}, fmt.Errorf("launch-session: exactly one of repository or workspace is required")
		}
		if name == "" {
			return ExecutionResult{}, fmt.Errorf("launch-session: session name is required")
		}
		if err := ValidateSessionName(name); err != nil {
			return ExecutionResult{}, fmt.Errorf("launch-session: session name: %w", err)
		}
		if repo != "" && input.Session.Agent != "" {
			agent = input.Session.Agent
		}
	} else if repo == "" && workspace == "" {
		if strings.Contains(cfg.RepoTemplate, ".ItemRemote") {
			return ExecutionResult{}, fmt.Errorf("launch-session: %s rendered blank: the item names no repository", repoField)
		}
		return ExecutionResult{}, fmt.Errorf("launch-session: %s rendered blank", repoField)
	}
	if repo != "" && workspace != "" {
		return ExecutionResult{}, fmt.Errorf("launch-session: repository and workspace targets are mutually exclusive")
	}
	if data.IsRerun {
		name = SessionNameWithSuffix(name, fmt.Sprintf("rerun-%d", data.CommandID))
		if err := ValidateSessionName(name); err != nil {
			return ExecutionResult{}, fmt.Errorf("launch-session: rerun session name: %w", err)
		}
	}

	var origins []models.ItemRef
	if data.Origin.Known() {
		origins = []models.ItemRef{data.Origin}
	}
	var outcome SessionExecutionOutcome
	if workspace != "" {
		outcome, err = e.launchWorkspace(ctx, LaunchWorkspaceSessionRequest{Workspace: workspace, Name: name, Prompt: prompt, Origins: origins})
	} else {
		outcome, err = e.launchRepository(ctx, LaunchSessionRequest{Name: name, Prompt: prompt, Agent: agent, Repo: repo, UniqueName: generatedName, Origins: origins})
	}
	if err != nil {
		return ExecutionResult{Attempted: true}, err
	}
	result := ExecutionResult{Attempted: true, Outcome: &ExecutionOutcome{Session: &outcome}}
	if repo != "" {
		result.Log = e.runPostHook(ctx, action, cfg, data, repo, outcome)
	}
	return result, nil
}

// sessionName renders the node's name template, falling back to a name
// derived from the action and the item when the template is empty or renders
// nothing a session name can keep.
func sessionName(actionID, nameTemplate string, data OutputData) (string, error) {
	var rendered string
	if strings.TrimSpace(nameTemplate) != "" {
		var err error
		rendered, err = tmpl.New(tmpl.Config{}).Render(nameTemplate, data)
		if err != nil {
			return "", fmt.Errorf("launch-session: session name: %w", err)
		}
	}
	name := ToSessionName(rendered, actionID+"-"+data.Key)
	if name == "" {
		return "", fmt.Errorf("launch-session: session name: neither the template nor the action id and item key have letters or digits")
	}
	return name, nil
}

func (e *LaunchSessionExecutor) launchRepository(ctx context.Context, req LaunchSessionRequest) (outcome SessionExecutionOutcome, err error) {
	if e.launcher == nil {
		return SessionExecutionOutcome{}, fmt.Errorf("launch-session executor: no repository session launcher configured")
	}
	ctx, span := observe.StartConditionalSpan(ctx, tracer, "dispatch.launch-session", trace.WithAttributes(
		attribute.String(attrAgent, req.Agent),
		attribute.String(attrRepo, req.Repo),
	))
	defer observe.End(span, &err)
	return e.launcher.LaunchSession(ctx, req)
}

func (e *LaunchSessionExecutor) launchWorkspace(ctx context.Context, req LaunchWorkspaceSessionRequest) (outcome SessionExecutionOutcome, err error) {
	if e.workspaceLauncher == nil {
		return SessionExecutionOutcome{}, fmt.Errorf("launch-session executor: no workspace session launcher configured")
	}
	ctx, span := observe.StartConditionalSpan(ctx, tracer, "dispatch.launch-session", trace.WithAttributes(
		attribute.String(attrWorkspace, req.Workspace),
	))
	defer observe.End(span, &err)
	return e.workspaceLauncher.LaunchWorkspaceSession(ctx, req)
}

// A failure stays in the log and never becomes the action's error: the session
// already exists, so a retry would create a second one.
func (e *LaunchSessionExecutor) runPostHook(
	ctx context.Context,
	action actions.Action,
	cfg *actions.LaunchSessionConfig,
	data OutputData,
	repo string,
	outcome SessionExecutionOutcome,
) ExecutionLog {
	if strings.TrimSpace(cfg.PostHook) == "" {
		return ExecutionLog{}
	}
	logger := e.logger.With().Str("action_id", action.ID).Str("session_id", outcome.ID).Logger()
	failed := func(err error) ExecutionLog {
		logger.Warn().Ctx(ctx).Err(err).Msg("launch-session: post hook failed")
		return ExecutionLog{Stderr: "post_hook: " + err.Error()}
	}
	if e.env == nil {
		return failed(errors.New("no execution environment configured"))
	}
	if outcome.Path == "" {
		return failed(errors.New("the launcher reported no checkout to run in"))
	}
	// Branch is absent: hive reports none for a fresh session, and a hook that
	// wants one is already a shell in the checkout.
	data.Session = &SessionTarget{ID: outcome.ID, Name: outcome.Name, Slug: outcome.Slug, Repo: repo, Path: outcome.Path}
	command, err := tmpl.New(tmpl.Config{}).Render(cfg.PostHook, data)
	if err != nil {
		return failed(err)
	}
	command = strings.TrimSpace(command)
	if command == "" {
		return failed(errors.New("rendered blank"))
	}
	timeout := cfg.PostHookTimeout.Duration()
	if timeout == 0 {
		timeout = defaultPostHookTimeout
	}
	log, err := runShell(ctx, e.env, "dispatch.post-hook", shellCommand{Command: command, Dir: outcome.Path, Timeout: timeout})
	if err != nil {
		logger.Warn().Ctx(ctx).Err(err).Msg("launch-session: post hook failed")
		log.Stderr = strings.TrimRight("post_hook: "+err.Error()+"\n"+log.Stderr, "\n")
		return log
	}
	logger.Info().Ctx(ctx).Msg("launch-session: post hook ran")
	return log
}
