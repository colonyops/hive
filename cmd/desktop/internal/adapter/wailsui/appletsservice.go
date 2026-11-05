package wailsui

import (
	"context"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
)

// AppletsService exposes the applet catalog and the bridge behind window.hive.
type AppletsService struct {
	applets *app.AppletsService
	chats   *app.AgentWorkspacesService
}

func NewAppletsService(a *app.AppletsService, chats *app.AgentWorkspacesService) *AppletsService {
	return &AppletsService{applets: a, chats: chats}
}

// AppletChatStart is hive.startChat's argument.
type AppletChatStart struct {
	Workspace string `json:"workspace"`
	Name      string `json:"name"`
	Prompt    string `json:"prompt"`
	PinToCode bool   `json:"pinToCode"`
}

// AppletChat is the chat a hive.startChat, hive.pinChat or hive.unpinChat
// call answers with.
type AppletChat struct {
	ID        string `json:"id"`
	Workspace string `json:"workspace"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	PinnedAt  int64  `json:"pinnedAt"`
}

func toAppletChat(v app.SessionView) AppletChat {
	return AppletChat{ID: v.ID, Workspace: v.Workspace, Name: v.Name, Slug: v.Slug, PinnedAt: v.PinnedAt}
}

// StartChat adds no authority: an applet's exec already runs any command as
// the user (ADR applets-are-web-pages-with-full-access-to-the-machine).
func (s *AppletsService) StartChat(ctx context.Context, id string, req AppletChatStart) (AppletChat, error) {
	if err := s.applets.CheckBridge(id); err != nil {
		return AppletChat{}, err
	}
	view, err := s.chats.CreateChat(ctx, app.CreateChat{
		Workspace: req.Workspace, Name: req.Name, Prompt: req.Prompt, PinToCode: req.PinToCode,
	})
	if err != nil {
		return AppletChat{}, err
	}
	return toAppletChat(view), nil
}

func (s *AppletsService) PinChat(ctx context.Context, id, chatID string, pinned bool) (AppletChat, error) {
	if err := s.applets.CheckBridge(id); err != nil {
		return AppletChat{}, err
	}
	view, err := s.chats.PinChat(ctx, chatID, pinned)
	if err != nil {
		return AppletChat{}, err
	}
	return toAppletChat(view), nil
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

func (s *AppletsService) Export(token, dest string) error {
	return s.applets.Export(token, dest)
}

func (s *AppletsService) CancelExport(token string) {
	s.applets.CancelExport(token)
}

func (s *AppletsService) SetEnabled(id string, enabled bool) error {
	return s.applets.SetEnabled(id, enabled)
}

func (s *AppletsService) SetSurface(id, surface string, on bool) error {
	return s.applets.SetSurface(id, surface, on)
}

func (s *AppletsService) Delete(id string) error {
	return s.applets.Delete(id)
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
