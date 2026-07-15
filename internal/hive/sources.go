package hive

import (
	"errors"
	"fmt"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/kv"
	"github.com/colonyops/hive/internal/sources"
	"github.com/colonyops/hive/internal/sources/cliengine"
	"github.com/colonyops/hive/internal/sources/ghcli"
	"github.com/colonyops/hive/internal/sources/rpc"
	"github.com/colonyops/hive/internal/sources/teacli"
	"github.com/rs/zerolog"
)

// registration is one source registration the registry should hold.
type registration struct {
	id          string
	backend     sources.Backend
	source      sources.Source
	templates   sources.TemplateConfig
	displayName string
}

// sourceEntry produces registry registrations from one configured source.
type sourceEntry interface {
	registrations(deps registryDeps) ([]registration, error)
}

// registryDeps contains the dependencies needed to construct configured
// sources; downstream registration remains expressed only in sources.Source.
type registryDeps struct {
	opts          cliengine.Options
	exec          cliengine.Executor
	kvStore       kv.KV
	processRunner rpc.ProcessRunner
}

// builtinEntry pairs a built-in source's config section with its per-forge
// drivers. The same source id resolves to gh or tea at picker-open time based
// on the repo's detected backend.
type builtinEntry struct {
	config  config.BuiltinSourceConfig
	drivers map[sources.Backend]cliengine.Driver
}

func (e builtinEntry) registrations(deps registryDeps) ([]registration, error) {
	if !isSourceEnabled(e.config.Enabled) {
		return nil, nil
	}

	templates := sourceTemplateConfig(e.config.Templates)
	registrations := make([]registration, 0, len(e.drivers))
	var constructionErrors []error
	for backend, driver := range e.drivers {
		driverCfg := driver.Config()
		source, err := cliengine.New(driver, deps.exec, deps.kvStore, deps.opts)
		if err != nil {
			constructionErrors = append(constructionErrors, fmt.Errorf("%s (%s): %w", driverCfg.ID, backend, err))
			continue
		}
		registrations = append(registrations, registration{
			id:          driverCfg.ID,
			backend:     backend,
			source:      source,
			templates:   templates,
			displayName: driverCfg.DisplayName,
		})
	}
	return registrations, errors.Join(constructionErrors...)
}

// viewEntry describes one saved GitHub search and the built-in configuration
// from which it inherits templates.
type viewEntry struct {
	name string
	cfg  config.SourceViewConfig
	base config.BuiltinSourceConfig
}

func (e viewEntry) registrations(deps registryDeps) ([]registration, error) {
	var driver cliengine.DetailDriver
	switch e.cfg.Base {
	case "issues":
		driver = ghcli.SearchIssues(e.name, e.name)
	case "prs":
		driver = ghcli.SearchPRs(e.name, e.name)
	default:
		return nil, fmt.Errorf("view %q: unsupported base %q", e.name, e.cfg.Base)
	}

	inner, err := cliengine.New(driver, deps.exec, deps.kvStore, deps.opts)
	if err != nil {
		return nil, fmt.Errorf("view %q: %w", e.name, err)
	}

	return []registration{{
		id:      e.name,
		backend: sources.BackendGithub,
		source: &viewSource{
			inner:       inner,
			displayName: e.name,
			query:       e.cfg.Query,
			scope:       e.cfg.Scope,
		},
		templates:   sourceTemplateConfig(e.base.Templates),
		displayName: e.name,
	}}, nil
}

// externalEntry describes one configured JSON-RPC subprocess source.
type externalEntry struct {
	cfg config.ExternalSourceConfig
}

func (e externalEntry) registrations(deps registryDeps) ([]registration, error) {
	registrations := make([]registration, 0, 2)
	for _, backend := range []sources.Backend{sources.BackendGithub, sources.BackendGitea} {
		// Construct one Source per backend rather than sharing a Source with a
		// mutable request counter (or an injectable runner) across picker flows.
		source, err := rpc.NewWithRunner(e.cfg, deps.processRunner)
		if err != nil {
			return nil, err
		}
		registrations = append(registrations, registration{
			id:          e.cfg.Name,
			backend:     backend,
			source:      source,
			templates:   sourceTemplateConfig(e.cfg.Templates),
			displayName: e.cfg.Name,
		})
	}
	return registrations, nil
}

// configSourceEntries returns built-ins first, followed by views and external
// sources in their respective config declaration order.
func configSourceEntries(cfg *config.Config) []sourceEntry {
	entries := []sourceEntry{
		builtinEntry{
			config: cfg.Sources.Issues,
			drivers: map[sources.Backend]cliengine.Driver{
				sources.BackendGithub: ghcli.Issues(),
				sources.BackendGitea:  teacli.Issues(),
			},
		},
		builtinEntry{
			config: cfg.Sources.PRs,
			drivers: map[sources.Backend]cliengine.Driver{
				sources.BackendGithub: ghcli.PRs(),
				sources.BackendGitea:  teacli.PRs(),
			},
		},
	}

	for _, view := range cfg.Sources.Views {
		base := cfg.Sources.Issues
		if view.Base == "prs" {
			base = cfg.Sources.PRs
		}
		entries = append(entries, viewEntry{name: view.Name, cfg: view, base: base})
	}
	for _, external := range cfg.Sources.External {
		entries = append(entries, externalEntry{cfg: external})
	}
	return entries
}

// BuildSourceRegistry constructs the sources.Registry from cfg. Construction
// and registration failures are logged and skipped rather than failing startup.
func BuildSourceRegistry(cfg *config.Config, exec cliengine.Executor, kvStore kv.KV, logger zerolog.Logger) *sources.Registry {
	return buildSourceRegistry(cfg, exec, kvStore, logger, rpc.ExecProcessRunner{})
}

// buildSourceRegistry keeps subprocess injection private to registry tests so
// the public startup seam does not grow another dependency.
func buildSourceRegistry(
	cfg *config.Config,
	exec cliengine.Executor,
	kvStore kv.KV,
	logger zerolog.Logger,
	processRunner rpc.ProcessRunner,
) *sources.Registry {
	registry := sources.NewRegistry()
	deps := registryDeps{
		opts: cliengine.Options{
			SearchLimit: cfg.Sources.SearchLimit,
			CacheTTL:    cfg.Sources.CacheTTL,
		},
		exec:          exec,
		kvStore:       kvStore,
		processRunner: processRunner,
	}

	for _, entry := range configSourceEntries(cfg) {
		registrations, err := entry.registrations(deps)
		if err != nil {
			logger.Warn().Err(err).Msg("sources: failed to construct configured source")
		}
		for _, registration := range registrations {
			if err := registry.Register(
				registration.id,
				registration.backend,
				registration.source,
				registration.templates,
				registration.displayName,
			); err != nil {
				logger.Warn().Err(err).
					Str("source", registration.id).
					Str("backend", registration.backend.String()).
					Msg("sources: failed to register configured source")
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
