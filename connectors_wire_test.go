package main

import (
	"sort"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/pkg/executil"
)

func boolPtr(b bool) *bool { return &b }

func TestBuildConnectorRegistry(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.ConnectorsConfig
		wantIDs []string
	}{
		{
			name:    "builtins enabled by default (nil)",
			cfg:     config.ConnectorsConfig{},
			wantIDs: []string{"issues", "prs"},
		},
		{
			name: "builtins explicitly enabled",
			cfg: config.ConnectorsConfig{
				Issues: config.BuiltinConnectorConfig{Enabled: boolPtr(true)},
				PRs:    config.BuiltinConnectorConfig{Enabled: boolPtr(true)},
			},
			wantIDs: []string{"issues", "prs"},
		},
		{
			name: "issues disabled leaves prs",
			cfg: config.ConnectorsConfig{
				Issues: config.BuiltinConnectorConfig{Enabled: boolPtr(false)},
			},
			wantIDs: []string{"prs"},
		},
		{
			name: "prs disabled leaves issues",
			cfg: config.ConnectorsConfig{
				PRs: config.BuiltinConnectorConfig{Enabled: boolPtr(false)},
			},
			wantIDs: []string{"issues"},
		},
		{
			name: "all builtins disabled",
			cfg: config.ConnectorsConfig{
				Issues: config.BuiltinConnectorConfig{Enabled: boolPtr(false)},
				PRs:    config.BuiltinConnectorConfig{Enabled: boolPtr(false)},
			},
			wantIDs: []string{},
		},
		{
			name: "external connector registered alongside builtins",
			cfg: config.ConnectorsConfig{
				External: []config.ExternalConnectorConfig{
					{ID: "jira", Command: []string{"jira-connector"}},
				},
			},
			wantIDs: []string{"issues", "jira", "prs"},
		},
		{
			name: "disabled external connector skipped",
			cfg: config.ConnectorsConfig{
				External: []config.ExternalConnectorConfig{
					{ID: "jira", Enabled: boolPtr(false), Command: []string{"jira-connector"}},
				},
			},
			wantIDs: []string{"issues", "prs"},
		},
		{
			name: "external id colliding with builtin is skipped, not fatal",
			cfg: config.ConnectorsConfig{
				External: []config.ExternalConnectorConfig{
					{ID: "issues", Command: []string{"my-issues"}},
				},
			},
			wantIDs: []string{"issues", "prs"},
		},
		{
			name: "external issues replaces builtin when builtin disabled",
			cfg: config.ConnectorsConfig{
				Issues: config.BuiltinConnectorConfig{Enabled: boolPtr(false)},
				External: []config.ExternalConnectorConfig{
					{ID: "issues", Command: []string{"my-issues"}},
				},
			},
			wantIDs: []string{"issues", "prs"},
		},
		{
			name: "invalid external entry (empty command) skipped without failing others",
			cfg: config.ConnectorsConfig{
				External: []config.ExternalConnectorConfig{
					{ID: "broken"},
					{ID: "jira", Command: []string{"jira-connector"}},
				},
			},
			wantIDs: []string{"issues", "jira", "prs"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{Connectors: tt.cfg}
			registry := buildConnectorRegistry(cfg, &executil.RealExecutor{}, nil, zerolog.Nop())
			require.NotNil(t, registry)

			ids := registry.IDs()
			sort.Strings(ids)
			assert.Equal(t, tt.wantIDs, ids)
		})
	}
}

func TestIsConnectorEnabled(t *testing.T) {
	assert.True(t, isConnectorEnabled(nil), "nil means enabled")
	assert.True(t, isConnectorEnabled(boolPtr(true)))
	assert.False(t, isConnectorEnabled(boolPtr(false)))
}
