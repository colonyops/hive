package wailsui

import (
	"context"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
)

// AppletsService exposes the applet catalog and the bridge behind window.hive.
type AppletsService struct {
	applets *app.AppletsService
}

func NewAppletsService(a *app.AppletsService) *AppletsService {
	return &AppletsService{applets: a}
}

func (s *AppletsService) List(ctx context.Context) []app.Applet {
	return s.applets.List(ctx)
}

func (s *AppletsService) Dir() string {
	return s.applets.Dir()
}

func (s *AppletsService) ExportPreview(id string) (app.AppletExportPreview, error) {
	return s.applets.ExportPreview(id)
}

func (s *AppletsService) Export(id, dest string) error {
	return s.applets.Export(id, dest)
}

func (s *AppletsService) PrepareImport(encoded string) (app.AppletImportPreview, error) {
	return s.applets.PrepareImport(encoded)
}

func (s *AppletsService) ConfirmImport(token, id string) error {
	return s.applets.ConfirmImport(token, id)
}

func (s *AppletsService) CancelImport(token string) {
	s.applets.CancelImport(token)
}

func (s *AppletsService) Processes() []app.AppletProcess {
	return s.applets.Processes()
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

func (s *AppletsService) KillSpawns(handles []string) {
	s.applets.KillSpawns(handles)
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
