package doctor

import (
	"context"

	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/session"
)

// ConfigValidator validates a program's whole config. Doctor takes the
// program's validator rather than the engine's so `hive doctor` still reports
// errors in sections only the CLI reads, such as keybindings.
type ConfigValidator interface {
	ValidateDeep(configPath string) error
	Warnings() []config.ValidationWarning
}

// Service runs health checks on the hive setup.
type Service struct {
	store       session.Store
	config      *config.Config
	validator   ConfigValidator
	pluginInfos []PluginInfo
}

// NewService creates a new Service.
func NewService(store session.Store, cfg *config.Config, validator ConfigValidator, pluginInfos []PluginInfo) *Service {
	return &Service{
		store:       store,
		config:      cfg,
		validator:   validator,
		pluginInfos: pluginInfos,
	}
}

// RunChecks executes all doctor checks and returns results.
func (d *Service) RunChecks(ctx context.Context, configPath string, autofix bool) []Result {
	checks := []Check{
		NewToolsCheck(),
		NewPluginCheck(d.pluginInfos),
		NewConfigCheck(func() error { return d.validator.ValidateDeep(configPath) }, d.validator.Warnings()),
		NewRepoDirsCheck(d.config.Workspaces),
		NewOrphanCheck(d.store, d.config.ReposDir(), autofix),
	}
	return RunAll(ctx, checks)
}
