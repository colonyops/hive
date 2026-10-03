package hive

import (
	"context"

	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/core/doctor"
	"github.com/colonyops/hive/internal/domain/session"
)

// ConfigValidator validates a program's whole config. Doctor takes the
// program's validator rather than the engine's so `hive doctor` still reports
// errors in sections only the CLI reads, such as keybindings.
type ConfigValidator interface {
	ValidateDeep(configPath string) error
	Warnings() []config.ValidationWarning
}

// DoctorService runs health checks on the hive setup.
type DoctorService struct {
	store       session.Store
	config      *config.Config
	validator   ConfigValidator
	pluginInfos []doctor.PluginInfo
}

// NewDoctorService creates a new DoctorService.
func NewDoctorService(store session.Store, cfg *config.Config, validator ConfigValidator, pluginInfos []doctor.PluginInfo) *DoctorService {
	return &DoctorService{
		store:       store,
		config:      cfg,
		validator:   validator,
		pluginInfos: pluginInfos,
	}
}

// RunChecks executes all doctor checks and returns results.
func (d *DoctorService) RunChecks(ctx context.Context, configPath string, autofix bool) []doctor.Result {
	checks := []doctor.Check{
		doctor.NewToolsCheck(),
		doctor.NewPluginCheck(d.pluginInfos),
		doctor.NewConfigCheck(func() error { return d.validator.ValidateDeep(configPath) }, d.validator.Warnings()),
		doctor.NewRepoDirsCheck(d.config.Workspaces),
		doctor.NewOrphanCheck(d.store, d.config.ReposDir(), autofix),
	}
	return doctor.RunAll(ctx, checks)
}
