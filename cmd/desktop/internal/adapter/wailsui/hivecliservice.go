package wailsui

import (
	"context"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
)

// HiveCLIService exposes the `hive` command install to first run and to
// Settings ▸ Hive CLI.
type HiveCLIService struct{ cli *app.HiveCLIService }

func NewHiveCLIService(cli *app.HiveCLIService) *HiveCLIService {
	return &HiveCLIService{cli: cli}
}

// Status reports the installed command, what the login shell resolves `hive`
// to, and how its version compares with the app's.
func (s *HiveCLIService) Status(ctx context.Context) (app.HiveCLIStatus, error) {
	return s.cli.Status(ctx)
}

// SetInstall saves the choice to install the command and applies it.
func (s *HiveCLIService) SetInstall(ctx context.Context, enabled bool) (app.HiveCLIStatus, error) {
	return s.cli.SetInstall(ctx, enabled)
}
