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

func TestBuildSourceRegistry(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.SourcesConfig
		wantIDs []string
	}{
		{
			name:    "builtins enabled by default (nil)",
			cfg:     config.SourcesConfig{},
			wantIDs: []string{"issues", "prs"},
		},
		{
			name: "builtins explicitly enabled",
			cfg: config.SourcesConfig{
				Issues: config.BuiltinSourceConfig{Enabled: boolPtr(true)},
				PRs:    config.BuiltinSourceConfig{Enabled: boolPtr(true)},
			},
			wantIDs: []string{"issues", "prs"},
		},
		{
			name: "issues disabled leaves prs",
			cfg: config.SourcesConfig{
				Issues: config.BuiltinSourceConfig{Enabled: boolPtr(false)},
			},
			wantIDs: []string{"prs"},
		},
		{
			name: "prs disabled leaves issues",
			cfg: config.SourcesConfig{
				PRs: config.BuiltinSourceConfig{Enabled: boolPtr(false)},
			},
			wantIDs: []string{"issues"},
		},
		{
			name: "all builtins disabled",
			cfg: config.SourcesConfig{
				Issues: config.BuiltinSourceConfig{Enabled: boolPtr(false)},
				PRs:    config.BuiltinSourceConfig{Enabled: boolPtr(false)},
			},
			wantIDs: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{Sources: tt.cfg}
			registry := buildSourceRegistry(cfg, &executil.RealExecutor{}, nil, zerolog.Nop())
			require.NotNil(t, registry)

			ids := registry.IDs()
			sort.Strings(ids)
			assert.Equal(t, tt.wantIDs, ids)
		})
	}
}

func TestIsSourceEnabled(t *testing.T) {
	assert.True(t, isSourceEnabled(nil), "nil means enabled")
	assert.True(t, isSourceEnabled(boolPtr(true)))
	assert.False(t, isSourceEnabled(boolPtr(false)))
}
