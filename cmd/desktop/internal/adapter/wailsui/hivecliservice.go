package wailsui

import (
	"context"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
)

// HiveCLIService exposes the `hive` command the app installs to first run and
// Settings ▸ Hive CLI.
type HiveCLIService struct{ cli *app.HiveCLIService }

func NewHiveCLIService(cli *app.HiveCLIService) *HiveCLIService {
	return &HiveCLIService{cli: cli}
}

// Status reports the command, what is at its install path, and what the
// login shell runs as `hive`.
func (s *HiveCLIService) Status(ctx context.Context) (app.HiveCLIStatus, error) {
	return s.cli.Status(ctx)
}

// SetInstall records whether the app installs the command and applies it.
func (s *HiveCLIService) SetInstall(ctx context.Context, install bool) (app.HiveCLIStatus, error) {
	return s.cli.SetInstall(ctx, install)
}
