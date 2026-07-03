package main

import (
	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/kv"
	"github.com/colonyops/hive/internal/sources"
	"github.com/colonyops/hive/internal/sources/ghcli"
	"github.com/colonyops/hive/pkg/executil"
	"github.com/rs/zerolog"
)

// builtinSources declares the gh-CLI-backed builtins: each entry pairs a
// declarative ghcli.Spec with the config section controlling it. Adding a
// new builtin means adding a Spec (see ghcli/issues.go, ghcli/prs.go), a
// config section, and one row here.
func builtinSources(cfg *config.Config) []struct {
	Spec   ghcli.Spec
	Config config.BuiltinSourceConfig
} {
	return []struct {
		Spec   ghcli.Spec
		Config config.BuiltinSourceConfig
	}{
		{Spec: ghcli.IssuesSpec(), Config: cfg.Sources.Issues},
		{Spec: ghcli.PRsSpec(), Config: cfg.Sources.PRs},
	}
}

// buildSourceRegistry constructs the sources.Registry from cfg. Registration
// failures are logged and the offending entry is skipped rather than failing
// startup.
func buildSourceRegistry(cfg *config.Config, exec executil.Executor, kvStore kv.KV, logger zerolog.Logger) *sources.Registry {
	registry := sources.NewRegistry()

	for _, builtin := range builtinSources(cfg) {
		if !isSourceEnabled(builtin.Config.Enabled) {
			continue
		}
		source, err := ghcli.New(builtin.Spec, exec, kvStore)
		if err != nil {
			logger.Warn().Err(err).Str("source", builtin.Spec.ID).Msg("sources: failed to construct builtin source")
			continue
		}
		templates := sourceTemplateConfig(builtin.Config.Templates)
		if err := registry.Register(source.Name(), source, templates, builtin.Spec.DisplayName); err != nil {
			logger.Warn().Err(err).Str("source", builtin.Spec.ID).Msg("sources: failed to register builtin source")
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
