package hive

import (
	"sort"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/kv"
	"github.com/colonyops/hive/internal/data/db"
	"github.com/colonyops/hive/internal/data/stores"
	"github.com/colonyops/hive/internal/sources"
	"github.com/colonyops/hive/pkg/executil"
)

func boolPtr(b bool) *bool { return &b }

func newSourcesTestKV(t *testing.T) kv.KV {
	t.Helper()
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	return stores.NewKVStore(database)
}

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
			registry := BuildSourceRegistry(cfg, &executil.RealExecutor{}, newSourcesTestKV(t), zerolog.Nop())
			require.NotNil(t, registry)

			ids := registry.IDs()
			sort.Strings(ids)
			assert.Equal(t, tt.wantIDs, ids)
		})
	}
}

func TestBuildSourceRegistryRegistersBothBackends(t *testing.T) {
	cfg := &config.Config{Sources: config.SourcesConfig{}}
	registry := BuildSourceRegistry(cfg, &executil.RealExecutor{}, newSourcesTestKV(t), zerolog.Nop())

	for _, backend := range []sources.Backend{sources.BackendGithub, sources.BackendGitea} {
		entries := registry.All(backend)
		ids := make([]string, 0, len(entries))
		for _, e := range entries {
			ids = append(ids, e.ID)
		}
		assert.Equal(t, []string{"issues", "prs"}, ids, "backend %s should service both builtins", backend)

		for _, id := range []string{"issues", "prs"} {
			_, _, ok := registry.Get(id, backend)
			assert.True(t, ok, "%s must be registered for backend %s", id, backend)
		}
	}
}

func TestIsSourceEnabled(t *testing.T) {
	assert.True(t, isSourceEnabled(nil), "nil means enabled")
	assert.True(t, isSourceEnabled(boolPtr(true)))
	assert.False(t, isSourceEnabled(boolPtr(false)))
}
