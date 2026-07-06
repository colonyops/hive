package hive

import (
	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/kv"
	"github.com/colonyops/hive/internal/sources"
	"github.com/colonyops/hive/internal/sources/cliengine"
	"github.com/colonyops/hive/internal/sources/ghcli"
	"github.com/colonyops/hive/internal/sources/teacli"
	"github.com/rs/zerolog"
)

// builtinSource pairs a built-in source's config section with its per-forge
// drivers. The same source id resolves to gh or tea at picker-open time based
// on the repo's detected backend.
type builtinSource struct {
	config  config.BuiltinSourceConfig
	drivers map[sources.Backend]cliengine.Driver
}

func builtinSources(cfg *config.Config) []builtinSource {
	return []builtinSource{
		{
			config: cfg.Sources.Issues,
			drivers: map[sources.Backend]cliengine.Driver{
				sources.BackendGithub: ghcli.Issues(),
				sources.BackendGitea:  teacli.Issues(),
			},
		},
		{
			config: cfg.Sources.PRs,
			drivers: map[sources.Backend]cliengine.Driver{
				sources.BackendGithub: ghcli.PRs(),
				sources.BackendGitea:  teacli.PRs(),
			},
		},
	}
}

// BuildSourceRegistry constructs the sources.Registry from cfg, registering a
// per-backend source for each enabled builtin. Registration failures are
// logged and the offending entry is skipped rather than failing startup.
func BuildSourceRegistry(cfg *config.Config, exec cliengine.Executor, kvStore kv.KV, logger zerolog.Logger) *sources.Registry {
	registry := sources.NewRegistry()

	opts := cliengine.Options{
		SearchLimit: cfg.Sources.SearchLimit,
		CacheTTL:    cfg.Sources.CacheTTL,
	}

	for _, builtin := range builtinSources(cfg) {
		if !isSourceEnabled(builtin.config.Enabled) {
			continue
		}
		templates := sourceTemplateConfig(builtin.config.Templates)
		for backend, driver := range builtin.drivers {
			driverCfg := driver.Config()
			source, err := cliengine.New(driver, exec, kvStore, opts)
			if err != nil {
				logger.Warn().Err(err).Str("source", driverCfg.ID).Str("backend", backend.String()).Msg("sources: failed to construct builtin source")
				continue
			}
			if err := registry.Register(driverCfg.ID, backend, source, templates, driverCfg.DisplayName); err != nil {
				logger.Warn().Err(err).Str("source", driverCfg.ID).Str("backend", backend.String()).Msg("sources: failed to register builtin source")
			}
		}
	}

	return registry
}

// isSourceEnabled treats nil as enabled; runtime availability is checked
// separately via Source.Available.
func isSourceEnabled(enabled *bool) bool {
	return enabled == nil || *enabled
}

func sourceTemplateConfig(cfg config.SourceTemplateConfig) sources.TemplateConfig {
	return sources.TemplateConfig{
		Name:   cfg.Name,
		Prompt: cfg.Prompt,
		Tags:   cfg.Tags,
	}
}
