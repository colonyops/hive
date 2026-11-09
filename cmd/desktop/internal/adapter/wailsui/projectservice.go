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

// ListServerProjects returns the project sessions on every server that
// answers, with "<server>:" in front of each id and slug.
func (s *SessionService) ListServerProjects(ctx context.Context) []app.ProjectView {
	return s.sessions.ListServerProjects(ctx)
}

func (s *SessionService) ProjectDefinitions(ctx context.Context, server string) ([]app.ProjectDefinitionView, error) {
	return s.sessions.ProjectDefinitions(ctx, server)
}

func (s *SessionService) DefaultCloneStrategy(ctx context.Context, server, remote string) (string, error) {
	return s.sessions.DefaultCloneStrategy(ctx, server, remote)
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
