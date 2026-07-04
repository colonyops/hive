package hive

import (
	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/kv"
	"github.com/colonyops/hive/internal/sources"
	"github.com/colonyops/hive/internal/sources/ghcli"
	"github.com/rs/zerolog"
)

// builtinSources pairs each gh-CLI-backed builtin driver with the config
// section controlling it. Adding a new builtin means adding a driver
// (see ghcli/issues.go, ghcli/prs.go), a config section, and one row here.
func builtinSources(cfg *config.Config) []struct {
	Driver ghcli.Driver
	Config config.BuiltinSourceConfig
} {
	return []struct {
		Driver ghcli.Driver
		Config config.BuiltinSourceConfig
	}{
		{Driver: ghcli.Issues(), Config: cfg.Sources.Issues},
		{Driver: ghcli.PRs(), Config: cfg.Sources.PRs},
	}
}

// BuildSourceRegistry constructs the sources.Registry from cfg.
// Registration failures are logged and the offending entry is skipped
// rather than failing startup.
func BuildSourceRegistry(cfg *config.Config, exec ghcli.Executor, kvStore kv.KV, logger zerolog.Logger) *sources.Registry {
	registry := sources.NewRegistry()

	opts := ghcli.Options{
		SearchLimit: cfg.Sources.SearchLimit,
		CacheTTL:    cfg.Sources.CacheTTL,
	}

	for _, builtin := range builtinSources(cfg) {
		if !isSourceEnabled(builtin.Config.Enabled) {
			continue
		}
		source, err := ghcli.New(builtin.Driver, exec, kvStore, opts)
		if err != nil {
			logger.Warn().Err(err).Str("source", builtin.Driver.Config().ID).Msg("sources: failed to construct builtin source")
			continue
		}
		templates := sourceTemplateConfig(builtin.Config.Templates)
		if err := registry.Register(source.Name(), source, templates, builtin.Driver.Config().DisplayName); err != nil {
			logger.Warn().Err(err).Str("source", builtin.Driver.Config().ID).Msg("sources: failed to register builtin source")
		}
	}

	return registry
}

// isSourceEnabled implements the nil = auto-detect, true/false = override
// convention used by plugin config: a nil Enabled defaults to enabled, and
// runtime availability is checked separately via Source.Available.
func isSourceEnabled(enabled *bool) bool {
	return enabled == nil || *enabled
}

// sourceTemplateConfig converts a config.SourceTemplateConfig into the
// sources.TemplateConfig shape used by RenderSessionTemplates.
func sourceTemplateConfig(cfg config.SourceTemplateConfig) sources.TemplateConfig {
	return sources.TemplateConfig{
		Name:   cfg.Name,
		Prompt: cfg.Prompt,
		Tags:   cfg.Tags,
	}
}
