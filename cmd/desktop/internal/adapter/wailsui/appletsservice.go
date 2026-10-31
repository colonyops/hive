package wailsui

import (
	"context"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/applets"
)

// AppletsService exposes the applet catalog and runs what applets declare.
type AppletsService struct {
	applets *app.AppletsService
}

func NewAppletsService(a *app.AppletsService) *AppletsService {
	return &AppletsService{applets: a}
}

func (s *AppletsService) List(ctx context.Context) []app.Applet {
	return s.applets.List(ctx)
}

func (s *AppletsService) Rows(ctx context.Context, id, sessionSlug string) (app.AppletRows, error) {
	return s.applets.Rows(ctx, id, sessionSlug)
}

func (s *AppletsService) Run(ctx context.Context, id, actionID, sessionSlug string, row applets.Row) (string, error) {
	return s.applets.Run(ctx, id, actionID, sessionSlug, row)
}

func (s *AppletsService) Context(ctx context.Context, id, sessionSlug string) (app.AppletContext, error) {
	return s.applets.Context(ctx, id, sessionSlug)
}

func (s *AppletsService) BridgeToken(id string) (string, error) {
	return s.applets.BridgeToken(id)
}

func (s *AppletsService) Exec(ctx context.Context, id string, req app.AppletExec) (app.AppletExecResult, error) {
	return s.applets.Exec(ctx, id, req)
}

func (s *AppletsService) Spawn(ctx context.Context, id string, req app.AppletExec) (string, error) {
	return s.applets.Spawn(ctx, id, req)
}

func (s *AppletsService) SpawnRead(ctx context.Context, handle string) (app.AppletOutput, error) {
	return s.applets.SpawnRead(ctx, handle)
}

func (s *AppletsService) SpawnWrite(handle, data string) error {
	return s.applets.SpawnWrite(handle, data)
}

func (s *AppletsService) SpawnKill(handle string) error {
	return s.applets.SpawnKill(handle)
}

func (s *AppletsService) KillSpawns(id string) {
	s.applets.KillSpawns(id)
}

func (s *AppletsService) ReadFile(id, path string) (string, error) {
	return s.applets.ReadFile(id, path)
}

func (s *AppletsService) WriteFile(id, path, content string) error {
	return s.applets.WriteFile(id, path, content)
}

func (s *AppletsService) SaveStorage(id, encoded string) error {
	return s.applets.SaveStorage(id, encoded)
}
