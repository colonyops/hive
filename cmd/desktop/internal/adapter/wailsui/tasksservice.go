package wailsui

import (
	"context"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
)

// TasksService exposes the desktop's tasks view over Hive's hc issue tracker.
type TasksService struct {
	tasks *app.TasksService
}

func NewTasksService(t *app.TasksService) *TasksService {
	return &TasksService{tasks: t}
}

func (s *TasksService) ListTasks(ctx context.Context, repoKey string) ([]TaskItem, error) {
	items, err := s.tasks.ListTasks(ctx, repoKey)
	if err != nil {
		return nil, err
	}
	return taskItemsOf(items), nil
}

func (s *TasksService) TaskDetail(ctx context.Context, id string) (TaskDetail, error) {
	detail, err := s.tasks.TaskDetail(ctx, id)
	if err != nil {
		return TaskDetail{}, err
	}
	return taskDetailOf(detail), nil
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
