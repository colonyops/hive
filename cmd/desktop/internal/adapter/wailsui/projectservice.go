package wailsui

import (
	"context"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
)

// ListProjects returns the project sessions on this machine. Slug is the tmux
// target an attach uses, as for a session.
func (s *SessionService) ListProjects(ctx context.Context) ([]app.ProjectView, error) {
	return s.sessions.ListProjects(ctx)
}

func (s *SessionService) ProjectDefinitions(ctx context.Context) []app.ProjectDefinitionView {
	return s.sessions.ProjectDefinitions(ctx)
}

func (s *SessionService) DefaultCloneStrategy(ctx context.Context, remote string) string {
	return s.sessions.DefaultCloneStrategy(ctx, remote)
}

// CreateProject starts a project create as a background job and returns its
// id. Progress per repository is read with ProjectCreateProgress after each
// projects:progress event.
func (s *SessionService) CreateProject(ctx context.Context, req app.CreateProjectRequest) (int64, error) {
	return s.sessions.CreateProject(ctx, req)
}

func (s *SessionService) ProjectCreateProgress(ctx context.Context) app.ProjectCreateProgress {
	return s.sessions.ProjectCreateProgress(ctx)
}

func (s *SessionService) ProjectRisk(ctx context.Context, id string) ([]app.ProjectRepoRisk, error) {
	return s.sessions.ProjectRisk(ctx, id)
}

func (s *SessionService) RecycleProject(ctx context.Context, id string) (int64, error) {
	return s.sessions.RecycleProject(ctx, id)
}

func (s *SessionService) DeleteProject(ctx context.Context, id string) (int64, error) {
	return s.sessions.DeleteProject(ctx, id)
}
