package commands

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/sources"
)

// fakeCmdSource is a sources.Source test double recording FetchDetail
// calls so tests can assert the capability gate.
type fakeCmdSource struct {
	available   bool
	fetchDetail bool
	items       []sources.Item
	detail      sources.Detail

	searchErr error
	detailErr error

	detailCalls []string
}

func (f *fakeCmdSource) Name() string                   { return "fake" }
func (f *fakeCmdSource) Available(context.Context) bool { return f.available }

func (f *fakeCmdSource) Initialize(context.Context) (sources.Manifest, error) {
	return sources.Manifest{
		ID:           "fake",
		Capabilities: sources.Capabilities{FetchDetail: f.fetchDetail},
	}, nil
}

func (f *fakeCmdSource) Search(_ context.Context, _ sources.SearchParams) (sources.SearchResult, error) {
	if f.searchErr != nil {
		return sources.SearchResult{}, f.searchErr
	}
	return sources.SearchResult{Items: f.items}, nil
}

func (f *fakeCmdSource) FetchDetail(_ context.Context, params sources.FetchDetailParams) (sources.Detail, error) {
	f.detailCalls = append(f.detailCalls, params.ID)
	if f.detailErr != nil {
		return sources.Detail{}, f.detailErr
	}
	return f.detail, nil
}

func newSourceTestRegistry(t *testing.T, src sources.Source) *sources.Registry {
	t.Helper()
	reg := sources.NewRegistry()
	require.NoError(t, reg.Register("fake", src, sources.TemplateConfig{
		Name:   "gh-{{ .Fields.number }}-{{ .Title }}",
		Prompt: "Work on {{ .Title }}\n\n{{ .Detail }}",
		Tags:   []string{"github"},
	}, "Fake"))
	return reg
}

func TestResolveSourceSessionRendersDetail(t *testing.T) {
	src := &fakeCmdSource{
		available:   true,
		fetchDetail: true,
		items: []sources.Item{{
			ID:     "42",
			Title:  "Fix the bug",
			Fields: map[string]any{"number": 42},
		}},
		detail: sources.Detail{Markdown: &sources.MarkdownDetail{Content: "issue body"}},
	}
	reg := newSourceTestRegistry(t, src)

	rendered, err := resolveSourceSession(context.Background(), reg, sourceOpenParams{
		SourceID: "fake",
		Pick:     "42",
		Scope:    "o/r",
	})
	require.NoError(t, err)

	assert.Equal(t, []string{"42"}, src.detailCalls, "detail must be fetched for capable sources")
	assert.Contains(t, rendered.Prompt, "Fix the bug")
	assert.Contains(t, rendered.Prompt, "issue body")
	assert.Equal(t, []string{"github"}, rendered.Tags)
	assert.NotEmpty(t, rendered.Name)
}

func TestResolveSourceSessionSkipsDetailWithoutCapability(t *testing.T) {
	src := &fakeCmdSource{
		available: true,
		items: []sources.Item{{
			ID:     "7",
			Title:  "A PR",
			Fields: map[string]any{"number": 7},
		}},
	}
	reg := newSourceTestRegistry(t, src)

	rendered, err := resolveSourceSession(context.Background(), reg, sourceOpenParams{
		SourceID: "fake",
		Pick:     "7",
	})
	require.NoError(t, err)
	assert.Empty(t, src.detailCalls, "detail must not be fetched without the capability")
	assert.Contains(t, rendered.Prompt, "A PR")
}

func TestResolveSourceSessionErrors(t *testing.T) {
	available := func() *fakeCmdSource {
		return &fakeCmdSource{
			available: true,
			items:     []sources.Item{{ID: "1", Title: "one"}},
		}
	}

	tests := []struct {
		name    string
		reg     func(t *testing.T) *sources.Registry
		params  sourceOpenParams
		wantErr string
	}{
		{
			name:    "missing source id",
			reg:     func(t *testing.T) *sources.Registry { return newSourceTestRegistry(t, available()) },
			params:  sourceOpenParams{Pick: "1"},
			wantErr: "source id is required",
		},
		{
			name:    "nil registry",
			reg:     func(*testing.T) *sources.Registry { return nil },
			params:  sourceOpenParams{SourceID: "fake", Pick: "1"},
			wantErr: "no sources are configured",
		},
		{
			name:    "unknown source",
			reg:     func(t *testing.T) *sources.Registry { return newSourceTestRegistry(t, available()) },
			params:  sourceOpenParams{SourceID: "nope", Pick: "1"},
			wantErr: `unknown source "nope"`,
		},
		{
			name: "unavailable source",
			reg: func(t *testing.T) *sources.Registry {
				src := available()
				src.available = false
				return newSourceTestRegistry(t, src)
			},
			params:  sourceOpenParams{SourceID: "fake", Pick: "1"},
			wantErr: "is not available",
		},
		{
			name: "search failure",
			reg: func(t *testing.T) *sources.Registry {
				src := available()
				src.searchErr = fmt.Errorf("rate limited")
				return newSourceTestRegistry(t, src)
			},
			params:  sourceOpenParams{SourceID: "fake", Pick: "1"},
			wantErr: "search: rate limited",
		},
		{
			name:    "pick not in results",
			reg:     func(t *testing.T) *sources.Registry { return newSourceTestRegistry(t, available()) },
			params:  sourceOpenParams{SourceID: "fake", Pick: "999"},
			wantErr: `no item with id "999"`,
		},
		{
			name: "detail fetch failure fails hard",
			reg: func(t *testing.T) *sources.Registry {
				src := available()
				src.fetchDetail = true
				src.detailErr = fmt.Errorf("boom")
				return newSourceTestRegistry(t, src)
			},
			params:  sourceOpenParams{SourceID: "fake", Pick: "1"},
			wantErr: "fetch detail",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resolveSourceSession(context.Background(), tt.reg(t), tt.params)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
