package tui

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/sources"
	"github.com/colonyops/hive/internal/tui/sourcepicker"
)

// stubSource is a minimal sources.Source for registry-backed wiring tests.
type stubSource struct {
	id        string
	detail    sources.Detail
	detailErr error
}

func (s stubSource) Name() string                   { return s.id }
func (s stubSource) Available(context.Context) bool { return true }
func (s stubSource) Initialize(context.Context) (sources.Manifest, error) {
	return sources.Manifest{ID: s.id}, nil
}

func (s stubSource) Search(context.Context, sources.SearchParams) (sources.SearchResult, error) {
	return sources.SearchResult{}, nil
}

func (s stubSource) FetchDetail(context.Context, sources.FetchDetailParams) (sources.Detail, error) {
	if s.detailErr != nil {
		return sources.Detail{}, s.detailErr
	}
	return s.detail, nil
}

func registryWith(t *testing.T, ids ...string) *sources.Registry {
	t.Helper()
	reg := sources.NewRegistry()
	for _, id := range ids {
		require.NoError(t, reg.Register(id, stubSource{id: id}, sources.TemplateConfig{}, id))
	}
	return reg
}

func TestResolveSourceID(t *testing.T) {
	tests := []struct {
		name    string
		reg     *sources.Registry
		args    []string
		want    string
		wantErr string
	}{
		{"explicit arg wins", registryWith(t, "issues", "prs"), []string{"prs"}, "prs", ""},
		{"explicit arg without registry", nil, []string{"issues"}, "issues", ""},
		{"sole source defaults", registryWith(t, "issues"), nil, "issues", ""},
		{"nil registry", nil, nil, "", "no sources are configured"},
		{"empty registry", registryWith(t), nil, "", "no sources are configured"},
		{"multiple sources are ambiguous", registryWith(t, "issues", "prs"), nil, "", "multiple sources configured"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Model{sourceRegistry: tt.reg}
			got, err := m.resolveSourceID(tt.args)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFetchSourceDetail(t *testing.T) {
	ctx := context.Background()
	body := sources.Detail{Markdown: &sources.MarkdownDetail{Content: "issue body"}}
	capable := sources.Manifest{Capabilities: sources.Capabilities{FetchDetail: true}}

	t.Run("fetches when capable and item has no detail", func(t *testing.T) {
		result := sourcepickerResult(stubSource{id: "issues", detail: body}, capable)
		got := fetchSourceDetail(ctx, result, "o/r")
		assert.Equal(t, body, got)
	})

	t.Run("no capability skips fetch", func(t *testing.T) {
		result := sourcepickerResult(stubSource{id: "prs", detail: body}, sources.Manifest{})
		got := fetchSourceDetail(ctx, result, "o/r")
		assert.Equal(t, sources.Detail{}, got)
	})

	t.Run("fetch failure degrades to empty detail", func(t *testing.T) {
		result := sourcepickerResult(stubSource{id: "issues", detailErr: assert.AnError}, capable)
		got := fetchSourceDetail(ctx, result, "o/r")
		assert.Equal(t, sources.Detail{}, got, "detail errors must not block session creation")
	})
}

func sourcepickerResult(src sources.Source, manifest sources.Manifest) sourcepicker.Result {
	return sourcepicker.Result{
		Item:     sources.Item{ID: "1", Title: "one"},
		SourceID: src.Name(),
		Source:   src,
		Manifest: manifest,
	}
}

func TestSourcePickerScopeForSelection(t *testing.T) {
	t.Run("explicit scope arg overrides selection", func(t *testing.T) {
		m := Model{}
		sess := &session.Session{Remote: "git@github.com:other/repo.git"}
		scope := m.sourcePickerScopeForSelection(sess, []string{"issues", "explicit/scope"})
		assert.Equal(t, "explicit/scope", scope.Search)
		assert.Empty(t, scope.Remote, "explicit scope without a discovered repo has no remote")
	})

	t.Run("selected session remote derives scope", func(t *testing.T) {
		m := Model{}
		sess := &session.Session{Remote: "git@github.com:myorg/myrepo.git"}
		scope := m.sourcePickerScopeForSelection(sess, nil)
		assert.Equal(t, "myorg/myrepo", scope.Search)
		assert.Equal(t, "git@github.com:myorg/myrepo.git", scope.Remote)
	})

	t.Run("no selection yields empty scope", func(t *testing.T) {
		m := Model{}
		scope := m.sourcePickerScopeForSelection(nil, nil)
		assert.Empty(t, scope.Search)
		assert.Empty(t, scope.Remote)
	})
}
