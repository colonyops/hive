package hive

import (
	"context"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/doctor"
	"github.com/colonyops/hive/internal/core/session"
)

// DoctorService runs health checks on the hive setup.
type DoctorService struct {
	store       session.Store
	config      *config.Config
	pluginInfos []doctor.PluginInfo
}

// NewDoctorService creates a new DoctorService.
func NewDoctorService(store session.Store, cfg *config.Config, pluginInfos []doctor.PluginInfo) *DoctorService {
	return &DoctorService{
		store:       store,
		config:      cfg,
		pluginInfos: pluginInfos,
	}
}

// RunChecks executes all doctor checks and returns results.
func (d *DoctorService) RunChecks(ctx context.Context, configPath string, autofix bool) []doctor.Result {
	checks := []doctor.Check{
		doctor.NewToolsCheck(),
		doctor.NewPluginCheck(d.pluginInfos),
		doctor.NewConfigCheck(d.config, configPath),
		doctor.NewWorkspacesCheck(d.config.Workspaces),
		doctor.NewOrphanCheck(d.store, d.config.ReposDir(), autofix),
	}
	return doctor.RunAll(ctx, checks)
}
