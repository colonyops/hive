package main

import (
	"time"

	"github.com/colonyops/hive/internal/connectors"
	connectorgithub "github.com/colonyops/hive/internal/connectors/github"
	"github.com/colonyops/hive/internal/connectors/rpc"
	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/kv"
	"github.com/colonyops/hive/pkg/executil"
	"github.com/rs/zerolog"
)

// defaultExternalConnectorTimeout bounds each RPC call made to an external
// connector subprocess.
const defaultExternalConnectorTimeout = 30 * time.Second

// buildConnectorRegistry constructs the connectors.Registry from cfg: the
// built-in GitHub issues connector (when enabled) and any configured
// external subprocess connectors. Registration failures (e.g. a duplicate
// external id) are logged and the offending entry is skipped rather than
// failing startup.
func buildConnectorRegistry(cfg *config.Config, exec executil.Executor, kvStore kv.KV, logger zerolog.Logger) *connectors.Registry {
	registry := connectors.NewRegistry()

	if isConnectorEnabled(cfg.Connectors.GitHub.Enabled) {
		conn := connectorgithub.New(exec, kvStore)
		templates := connectorTemplateConfig(cfg.Connectors.GitHub.Templates)
		if err := registry.Register(conn.Name(), conn, templates); err != nil {
			logger.Warn().Err(err).Msg("connectors: failed to register github connector")
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
