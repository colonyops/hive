package wailsui

import (
	"context"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
)

// ActionsService is the explicit editor API for the global actions catalog.
type ActionsService struct {
	actions *app.ActionsService
}

func NewActionsService(catalog *app.ActionsService) *ActionsService {
	return &ActionsService{actions: catalog}
}

func (s *ActionsService) ListActions(ctx context.Context) actions.EditableCatalog {
	return s.actions.List(ctx)
}

func (s *ActionsService) CreateAction(ctx context.Context, a actions.EditableAction) (actions.EditableAction, error) {
	return s.actions.Create(ctx, a)
}

func (s *ActionsService) UpdateAction(ctx context.Context, id string, a actions.EditableAction) (actions.EditableAction, error) {
	return s.actions.Update(ctx, id, a)
}

// ReorderActions persists the catalog order the settings list was dragged
// into. ids must be the full catalog.
func (s *ActionsService) ReorderActions(ctx context.Context, ids []string) error {
	return s.actions.Reorder(ctx, ids)
}

func (s *ActionsService) DeleteAction(ctx context.Context, id string) error {
	return s.actions.Delete(ctx, id)
}

// The launchers are the other list in actions.yml, so ListActions already
// carries them and only the writes need methods of their own.

func (s *ActionsService) CreateLauncher(ctx context.Context, l actions.Launcher) (actions.Launcher, error) {
	return s.actions.CreateLauncher(ctx, l)
}

func (s *ActionsService) UpdateLauncher(ctx context.Context, id string, l actions.Launcher) (actions.Launcher, error) {
	return s.actions.UpdateLauncher(ctx, id, l)
}

func (s *ActionsService) DeleteLauncher(ctx context.Context, id string) error {
	return s.actions.DeleteLauncher(ctx, id)
}
