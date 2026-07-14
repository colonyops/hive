package hive

import (
	"context"
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
	"github.com/colonyops/hive/pkg/executil/executiltest"
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

func TestConfigSourceEntriesOrder(t *testing.T) {
	cfg := &config.Config{Sources: config.SourcesConfig{
		Views: []config.SourceViewConfig{
			{Name: "triage", Base: "issues", Query: "label:triage"},
			{Name: "reviews", Base: "prs", Query: "review-requested:@me"},
		},
	}}

	entries := configSourceEntries(cfg)
	require.Len(t, entries, 4)

	issues, ok := entries[0].(builtinEntry)
	require.True(t, ok)
	assert.Equal(t, "issues", issues.drivers[sources.BackendGithub].Config().ID)
	prs, ok := entries[1].(builtinEntry)
	require.True(t, ok)
	assert.Equal(t, "prs", prs.drivers[sources.BackendGithub].Config().ID)

	triage, ok := entries[2].(viewEntry)
	require.True(t, ok)
	assert.Equal(t, "triage", triage.name)
	reviews, ok := entries[3].(viewEntry)
	require.True(t, ok)
	assert.Equal(t, "reviews", reviews.name)
}

func TestBuildSourceRegistryRegistersGithubViews(t *testing.T) {
	cfg := &config.Config{Sources: config.SourcesConfig{
		Issues: config.BuiltinSourceConfig{Templates: config.SourceTemplateConfig{
			Name: "issue-{{ .Fields.number }}", Prompt: "issue prompt", Tags: []string{"issue"},
		}},
		PRs: config.BuiltinSourceConfig{Templates: config.SourceTemplateConfig{
			Name: "pr-{{ .Fields.number }}", Prompt: "pr prompt", Tags: []string{"review"},
		}},
		Views: []config.SourceViewConfig{
			{Name: "triage", Base: "issues", Query: "label:triage"},
			{Name: "reviews", Base: "prs", Query: "review-requested:@me"},
		},
	}}

	registry := BuildSourceRegistry(cfg, &executil.RealExecutor{}, newSourcesTestKV(t), zerolog.Nop())
	githubEntries := registry.All(sources.BackendGithub)
	require.Len(t, githubEntries, 4)
	assert.Equal(t, []string{"issues", "prs", "triage", "reviews"}, registryEntryIDs(githubEntries))
	assert.Equal(t, "triage", githubEntries[2].Source.Name())
	assert.Equal(t, "reviews", githubEntries[3].Source.Name())
	assert.Equal(t, sources.TemplateConfig{
		Name: "issue-{{ .Fields.number }}", Prompt: "issue prompt", Tags: []string{"issue"},
	}, githubEntries[2].Templates)
	assert.Equal(t, sources.TemplateConfig{
		Name: "pr-{{ .Fields.number }}", Prompt: "pr prompt", Tags: []string{"review"},
	}, githubEntries[3].Templates)

	giteaEntries := registry.All(sources.BackendGitea)
	assert.Equal(t, []string{"issues", "prs"}, registryEntryIDs(giteaEntries))
	_, _, ok := registry.Get("triage", sources.BackendGitea)
	assert.False(t, ok)
	_, _, ok = registry.Get("reviews", sources.BackendGitea)
	assert.False(t, ok)
}

func TestBuildSourceRegistrySeparatesViewCaches(t *testing.T) {
	viewAResult := []byte(`[{"number":1,"title":"From A","state":"OPEN","author":{"login":"alice"},"labels":[],"url":"https://github.com/owner/repo/pull/1","createdAt":"2026-07-01T00:00:00Z","assignees":[],"isDraft":false,"repository":{"nameWithOwner":"owner/repo"}}]`)
	viewBResult := []byte(`[{"number":2,"title":"From B","state":"OPEN","author":{"login":"bob"},"labels":[],"url":"https://github.com/owner/repo/pull/2","createdAt":"2026-07-02T00:00:00Z","assignees":[],"isDraft":false,"repository":{"nameWithOwner":"owner/repo"}}]`)
	exec := &executiltest.Exec{Responses: []executiltest.Response{{Out: viewAResult}, {Out: viewBResult}}}
	cfg := &config.Config{Sources: config.SourcesConfig{
		Views: []config.SourceViewConfig{
			{Name: "reviews-a", Base: "prs", Query: "review-requested:@me", Scope: "owner/repo"},
			{Name: "reviews-b", Base: "prs", Query: "review-requested:@me", Scope: "owner/repo"},
		},
	}}
	registry := BuildSourceRegistry(cfg, exec, newSourcesTestKV(t), zerolog.Nop())
	viewA, _, ok := registry.Get("reviews-a", sources.BackendGithub)
	require.True(t, ok)
	viewB, _, ok := registry.Get("reviews-b", sources.BackendGithub)
	require.True(t, ok)

	resultA, err := viewA.Search(context.Background(), sources.SearchParams{Query: "caller-a", Scope: "wrong/a"})
	require.NoError(t, err)
	require.Len(t, resultA.Items, 1)
	assert.Equal(t, "From A", resultA.Items[0].Title)

	resultB, err := viewB.Search(context.Background(), sources.SearchParams{Query: "caller-b", Scope: "wrong/b"})
	require.NoError(t, err)
	require.Len(t, resultB.Items, 1)
	assert.Equal(t, "From B", resultB.Items[0].Title)

	cachedA, err := viewA.Search(context.Background(), sources.SearchParams{})
	require.NoError(t, err)
	require.Len(t, cachedA.Items, 1)
	assert.Equal(t, "From A", cachedA.Items[0].Title)
	assert.Len(t, exec.Calls(), 2, "view A's cache entry must not satisfy view B")
}

func TestBuildSourceRegistrySkipsInvalidViews(t *testing.T) {
	cfg := &config.Config{Sources: config.SourcesConfig{Views: []config.SourceViewConfig{
		{Name: "broken", Base: "unknown", Query: "anything"},
		{Name: "issues", Base: "issues", Query: "duplicate registry id"},
		{Name: "working", Base: "issues", Query: "is:open"},
	}}}

	registry := BuildSourceRegistry(cfg, &executil.RealExecutor{}, newSourcesTestKV(t), zerolog.Nop())
	assert.Equal(t, []string{"issues", "prs", "working"}, registry.IDs())
	_, _, ok := registry.Get("broken", sources.BackendGithub)
	assert.False(t, ok)
	_, _, ok = registry.Get("working", sources.BackendGithub)
	assert.True(t, ok)
}

func registryEntryIDs(entries []sources.RegistryEntry) []string {
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.ID)
	}
	return ids
}

func TestIsSourceEnabled(t *testing.T) {
	assert.True(t, isSourceEnabled(nil), "nil means enabled")
	assert.True(t, isSourceEnabled(boolPtr(true)))
	assert.False(t, isSourceEnabled(boolPtr(false)))
}
