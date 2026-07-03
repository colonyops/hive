package main

import (
	"time"

	"github.com/colonyops/hive/internal/connectors"
	"github.com/colonyops/hive/internal/connectors/ghcli"
	"github.com/colonyops/hive/internal/connectors/rpc"
	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/kv"
	"github.com/colonyops/hive/pkg/executil"
	"github.com/rs/zerolog"
)

// defaultExternalConnectorTimeout bounds each RPC call made to an external
// connector subprocess.
const defaultExternalConnectorTimeout = 30 * time.Second

// builtinConnectors declares the gh-CLI-backed builtins: each entry pairs
// a declarative ghcli.Spec with the config section controlling it. Adding
// a new builtin means adding a Spec (see ghcli/issues.go, ghcli/prs.go), a
// config section, and one row here.
func builtinConnectors(cfg *config.Config) []struct {
	Spec   ghcli.Spec
	Config config.BuiltinConnectorConfig
} {
	return []struct {
		Spec   ghcli.Spec
		Config config.BuiltinConnectorConfig
	}{
		{Spec: ghcli.IssuesSpec(), Config: cfg.Connectors.Issues},
		{Spec: ghcli.PRsSpec(), Config: cfg.Connectors.PRs},
	}
}

// buildConnectorRegistry constructs the connectors.Registry from cfg: the
// built-in gh-CLI connectors (when enabled) and any configured external
// subprocess connectors. Registration failures (e.g. a duplicate external
// id) are logged and the offending entry is skipped rather than failing
// startup.
func buildConnectorRegistry(cfg *config.Config, exec executil.Executor, kvStore kv.KV, logger zerolog.Logger) *connectors.Registry {
	registry := connectors.NewRegistry()

	for _, builtin := range builtinConnectors(cfg) {
		if !isConnectorEnabled(builtin.Config.Enabled) {
			continue
		}
		conn, err := ghcli.New(builtin.Spec, exec, kvStore)
		if err != nil {
			logger.Warn().Err(err).Str("connector", builtin.Spec.ID).Msg("connectors: failed to construct builtin connector")
			continue
		}
		templates := connectorTemplateConfig(builtin.Config.Templates)
		if err := registry.Register(conn.Name(), conn, templates); err != nil {
			logger.Warn().Err(err).Str("connector", builtin.Spec.ID).Msg("connectors: failed to register builtin connector")
		}
	}

	for _, ext := range cfg.Connectors.External {
		if !isConnectorEnabled(ext.Enabled) {
			continue
		}
		conn, err := rpc.NewSubprocessConnector(ext.ID, ext.Command, rpc.ExecProcessRunner{}, defaultExternalConnectorTimeout)
		if err != nil {
			logger.Warn().Err(err).Str("connector", ext.ID).Msg("connectors: failed to construct external connector")
			continue
		}
		templates := connectorTemplateConfig(ext.Templates)
		if err := registry.Register(ext.ID, conn, templates); err != nil {
			logger.Warn().Err(err).Str("connector", ext.ID).Msg("connectors: failed to register external connector")
		}
	}

	return registry
}

// isConnectorEnabled implements the nil = auto-detect, true/false = override
// convention used by plugin config: a nil Enabled defaults to enabled, and
// runtime availability is checked separately via Connector.Available.
func isConnectorEnabled(enabled *bool) bool {
	return enabled == nil || *enabled
}

// connectorTemplateConfig converts a config.ConnectorTemplateConfig into the
// connectors.TemplateConfig shape used by RenderSessionTemplates.
func connectorTemplateConfig(cfg config.ConnectorTemplateConfig) connectors.TemplateConfig {
	return connectors.TemplateConfig{
		Name:   cfg.Name,
		Prompt: cfg.Prompt,
		Tags:   cfg.Tags,
	}
}
