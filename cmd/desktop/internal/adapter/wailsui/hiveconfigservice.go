package wailsui

import (
	"context"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/hiveconf"
)

// HiveConfigService exposes the external Hive CLI configuration to first run
// and to Settings ▸ Hive CLI: which agent starts a session and which folders
// hold the repositories the session picker offers.
type HiveConfigService struct{ hive *app.HiveConfigService }

func NewHiveConfigService(hive *app.HiveConfigService) *HiveConfigService {
	return &HiveConfigService{hive: hive}
}

// Setup reports what the Hive config declares and which agents are installed.
func (s *HiveConfigService) Setup(ctx context.Context) app.HiveSetup {
	return s.hive.Setup(ctx)
}

// InspectWorkspace validates a chosen folder and counts the repositories in
// it, so the picker can confirm the choice before it is saved.
func (s *HiveConfigService) InspectWorkspace(ctx context.Context, path string) (hiveconf.Workspace, error) {
	return s.hive.InspectWorkspace(ctx, path)
}

// Save writes the configuration and reloads the running Hive services from it.
func (s *HiveConfigService) Save(ctx context.Context, req app.HiveSetupRequest) (app.HiveSetup, error) {
	return s.hive.Save(ctx, req)
}
