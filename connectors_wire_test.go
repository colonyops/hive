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
			name:    "github enabled by default (nil)",
			cfg:     config.ConnectorsConfig{},
			wantIDs: []string{"github"},
		},
		{
			name: "github explicitly enabled",
			cfg: config.ConnectorsConfig{
				GitHub: config.GitHubConnectorConfig{Enabled: boolPtr(true)},
			},
			wantIDs: []string{"github"},
		},
		{
			name: "github disabled",
			cfg: config.ConnectorsConfig{
				GitHub: config.GitHubConnectorConfig{Enabled: boolPtr(false)},
			},
			wantIDs: []string{},
		},
		{
			name: "external connector registered",
			cfg: config.ConnectorsConfig{
				External: []config.ExternalConnectorConfig{
					{ID: "jira", Command: []string{"jira-connector"}},
				},
			},
			wantIDs: []string{"github", "jira"},
		},
		{
			name: "disabled external connector skipped",
			cfg: config.ConnectorsConfig{
				External: []config.ExternalConnectorConfig{
					{ID: "jira", Enabled: boolPtr(false), Command: []string{"jira-connector"}},
				},
			},
			wantIDs: []string{"github"},
		},
		{
			name: "external id colliding with built-in github is skipped, not fatal",
			cfg: config.ConnectorsConfig{
				External: []config.ExternalConnectorConfig{
					{ID: "github", Command: []string{"my-github"}},
				},
			},
			wantIDs: []string{"github"},
		},
		{
			name: "external github replaces built-in when built-in disabled",
			cfg: config.ConnectorsConfig{
				GitHub: config.GitHubConnectorConfig{Enabled: boolPtr(false)},
				External: []config.ExternalConnectorConfig{
					{ID: "github", Command: []string{"my-github"}},
				},
			},
			wantIDs: []string{"github"},
		},
		{
			name: "invalid external entry (empty command) skipped without failing others",
			cfg: config.ConnectorsConfig{
				External: []config.ExternalConnectorConfig{
					{ID: "broken"},
					{ID: "jira", Command: []string{"jira-connector"}},
				},
			},
			wantIDs: []string{"github", "jira"},
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
