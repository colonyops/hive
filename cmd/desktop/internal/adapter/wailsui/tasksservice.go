package wailsui

import (
	"context"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/dispatch"
)

// TasksService exposes the desktop's tasks view over Hive's hc issue tracker.
type TasksService struct {
	tasks *app.TasksService
}

func NewTasksService(t *app.TasksService) *TasksService {
	return &TasksService{tasks: t}
}

func (s *TasksService) ListTasks(ctx context.Context, repoKey string) ([]dispatch.TaskItem, error) {
	return s.tasks.ListTasks(ctx, repoKey)
}

func (s *TasksService) TaskDetail(ctx context.Context, id string) (dispatch.TaskDetail, error) {
	return s.tasks.TaskDetail(ctx, id)
}

func (s *TasksService) SetTaskStatus(ctx context.Context, id, status string) error {
	return s.tasks.SetTaskStatus(ctx, id, status)
}

func (s *TasksService) DeleteTask(ctx context.Context, id string) error {
	return s.tasks.DeleteTask(ctx, id)
}

func (s *TasksService) PruneTasks(ctx context.Context, olderThanDays int, repoKey string, dryRun bool) (int, error) {
	return s.tasks.PruneTasks(ctx, olderThanDays, repoKey, dryRun)
}

func (s *TasksService) TaskRepoKeys(ctx context.Context) ([]string, error) {
	return s.tasks.TaskRepoKeys(ctx)
}
