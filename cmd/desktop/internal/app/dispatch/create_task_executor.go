package dispatch

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
	"github.com/colonyops/hive/internal/domain/hc"
	"github.com/colonyops/hive/internal/platform/observe"
	"github.com/colonyops/hive/pkg/tmpl"
)

// ActionTypeCreateTask is never authored in actions.yml: only a flow
// create-task node enqueues it.
const ActionTypeCreateTask = "create-task"

// ActionTypePublishMessage is the actions.yml publish-message type, which a
// flow publish-message node reuses.
const ActionTypePublishMessage = "publish-message"

// CreateTaskActionConfig is a create-task node projected onto the executor. It
// is rebuilt on every resolution, never persisted.
type CreateTaskActionConfig struct {
	TitleTemplate       string
	DescriptionTemplate string
	RepoTemplate        string
	Parent              string
}

func (c *CreateTaskActionConfig) Validate() error {
	if strings.TrimSpace(c.TitleTemplate) == "" {
		return fmt.Errorf("create-task: title is required")
	}
	return nil
}

// TaskCreator is satisfied by *hcsvc.Service.
type TaskCreator interface {
	CreateItem(ctx context.Context, repoKey string, input hc.CreateItemInput) (hc.Item, error)
}

// repoKeyPattern is the "owner/repo" shape hc derives from a git remote. A
// rendered repo outside it would file the task under a key no repository
// lists.
var repoKeyPattern = regexp.MustCompile(`^[^/\s]+/[^/\s]+$`)

// CreateTaskExecutor calls tasks once per execution, so a hive config reload
// reaches the next task.
type CreateTaskExecutor struct{ tasks func() TaskCreator }

func NewCreateTaskExecutor(tasks func() TaskCreator) *CreateTaskExecutor {
	return &CreateTaskExecutor{tasks: tasks}
}

func (e *CreateTaskExecutor) Execute(ctx context.Context, action actions.Action, data OutputData, _ ActionInvocationInput) (ExecutionResult, error) {
	cfg, ok := action.Config.(*CreateTaskActionConfig)
	if !ok {
		return ExecutionResult{}, fmt.Errorf("create-task executor: action %q has config type %T", action.ID, action.Config)
	}
	if e.tasks == nil {
		return ExecutionResult{}, fmt.Errorf("create-task executor: no task service configured")
	}
	renderer := tmpl.New(tmpl.Config{})
	title, err := renderer.Render(cfg.TitleTemplate, data)
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("create-task: title: %w", err)
	}
	// hc titles are one line; a payload field with a newline must not split it.
	title = strings.Join(strings.Fields(title), " ")
	if title == "" {
		return ExecutionResult{}, fmt.Errorf("create-task: title rendered blank")
	}
	desc, err := renderer.Render(cfg.DescriptionTemplate, data)
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("create-task: description: %w", err)
	}
	repo, err := renderer.Render(cfg.RepoTemplate, data)
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("create-task: repo: %w", err)
	}
	repo = strings.TrimSpace(repo)
	if repo != "" && !repoKeyPattern.MatchString(repo) {
		return ExecutionResult{}, fmt.Errorf("create-task: repo rendered %q, want owner/repo", repo)
	}

	runLog := RunLogFrom(ctx)
	runLog.Systemf("Creating task %q", title)
	item, err := e.create(ctx, repo, hc.CreateItemInput{
		Title: title, Desc: strings.TrimSpace(desc), Type: hc.ItemTypeTask, ParentID: cfg.Parent,
	})
	if err != nil {
		return ExecutionResult{Attempted: true}, err
	}
	if item.RepoKey != "" {
		runLog.Systemf("Created task %s in %s", item.ID, item.RepoKey)
	} else {
		runLog.Systemf("Created task %s", item.ID)
	}
	return ExecutionResult{Attempted: true}, nil
}

func (e *CreateTaskExecutor) create(ctx context.Context, repo string, input hc.CreateItemInput) (item hc.Item, err error) {
	ctx, span := observe.StartConditionalSpan(ctx, tracer, "dispatch.create-task", trace.WithAttributes(
		attribute.String(attrTaskRepo, repo),
	))
	defer observe.End(span, &err)

	item, err = e.tasks().CreateItem(ctx, repo, input)
	if err != nil {
		return hc.Item{}, fmt.Errorf("create task: %w", err)
	}
	return item, nil
}
