package connectors_test

import (
	"context"
	"testing"

	"github.com/colonyops/hive/internal/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeConnector is a minimal in-memory Connector implementation shared by
// tests in this package and later phases (TUI, registry).
type fakeConnector struct {
	name      string
	available bool
	manifest  connectors.Manifest
	items     []connectors.Item
	detail    connectors.Detail

	initErr   error
	searchErr error
	detailErr error
}

func (f *fakeConnector) Name() string { return f.name }

func (f *fakeConnector) Available(_ context.Context) bool { return f.available }

func (f *fakeConnector) Initialize(_ context.Context) (connectors.Manifest, error) {
	if f.initErr != nil {
		return connectors.Manifest{}, f.initErr
	}
	return f.manifest, nil
}

func (f *fakeConnector) Search(_ context.Context, _ connectors.SearchParams) (connectors.SearchResult, error) {
	if f.searchErr != nil {
		return connectors.SearchResult{}, f.searchErr
	}
	return connectors.SearchResult{Items: f.items}, nil
}

func (f *fakeConnector) FetchDetail(_ context.Context, _ connectors.FetchDetailParams) (connectors.Detail, error) {
	if f.detailErr != nil {
		return connectors.Detail{}, f.detailErr
	}
	return f.detail, nil
}

var _ connectors.Connector = (*fakeConnector)(nil)

func TestRenderSessionTemplates(t *testing.T) {
	tests := []struct {
		name       string
		cfg        connectors.TemplateConfig
		item       connectors.Item
		detail     connectors.Detail
		wantName   string
		wantPrompt string
		wantTags   []string
		wantErr    string
	}{
		{
			name: "renders name prompt and tags from fields",
			cfg: connectors.TemplateConfig{
				Name:   "gh-{{ .Fields.number }}-{{ .Title }}",
				Prompt: "Work on {{ .Title }}\n\n{{ .Fields.url }}",
				Tags:   []string{"github", "issue-{{ .Fields.number }}"},
			},
			item: connectors.Item{
				ID:    "1",
				Title: "Fix bug",
				Fields: map[string]any{
					"number": 42,
					"url":    "https://example.com/1",
				},
			},
			wantName:   "gh-42-Fix bug",
			wantPrompt: "Work on Fix bug\n\nhttps://example.com/1",
			wantTags:   []string{"github", "issue-42"},
		},
		{
			name: "renders detail content",
			cfg: connectors.TemplateConfig{
				Name:   "{{ .ID }}",
				Prompt: "{{ .Detail }}",
			},
			item:       connectors.Item{ID: "1", Title: "Item"},
			detail:     connectors.Detail{Markdown: &connectors.MarkdownDetail{Content: "body text"}},
			wantName:   "1",
			wantPrompt: "body text",
			wantTags:   []string{},
		},
		{
			name: "missing field errors on name template",
			cfg: connectors.TemplateConfig{
				Name:   "{{ .Fields.missing }}",
				Prompt: "ok",
			},
			item:    connectors.Item{ID: "1", Title: "Item", Fields: map[string]any{}},
			wantErr: "name template",
		},
		{
			name: "missing field errors on prompt template",
			cfg: connectors.TemplateConfig{
				Name:   "ok",
				Prompt: "{{ .Fields.missing }}",
			},
			item:    connectors.Item{ID: "1", Title: "Item", Fields: map[string]any{}},
			wantErr: "prompt template",
		},
		{
			name: "missing field errors on tags template",
			cfg: connectors.TemplateConfig{
				Name:   "ok",
				Prompt: "ok",
				Tags:   []string{"{{ .Fields.missing }}"},
			},
			item:    connectors.Item{ID: "1", Title: "Item", Fields: map[string]any{}},
			wantErr: "tags[0] template",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := connectors.RenderSessionTemplates(tt.cfg, tt.item, tt.detail)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantName, got.Name)
			assert.Equal(t, tt.wantPrompt, got.Prompt)
			assert.Equal(t, tt.wantTags, got.Tags)
		})
	}
}

func TestDetail_KindAndValid(t *testing.T) {
	tests := []struct {
		name      string
		detail    connectors.Detail
		wantKind  connectors.DetailKind
		wantValid bool
	}{
		{
			name:      "none",
			detail:    connectors.Detail{},
			wantKind:  connectors.DetailKindNone,
			wantValid: true,
		},
		{
			name:      "markdown",
			detail:    connectors.Detail{Markdown: &connectors.MarkdownDetail{Content: "x"}},
			wantKind:  connectors.DetailKindMarkdown,
			wantValid: true,
		},
		{
			name:      "kv",
			detail:    connectors.Detail{KV: &connectors.KVDetail{}},
			wantKind:  connectors.DetailKindKV,
			wantValid: true,
		},
		{
			name: "both set is invalid",
			detail: connectors.Detail{
				Markdown: &connectors.MarkdownDetail{Content: "x"},
				KV:       &connectors.KVDetail{},
			},
			wantKind:  connectors.DetailKindMarkdown,
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantKind, tt.detail.Kind())
			assert.Equal(t, tt.wantValid, tt.detail.Valid())
		})
	}
}

func TestFakeConnector_ImplementsInterface(t *testing.T) {
	fc := &fakeConnector{
		name:      "fake",
		available: true,
		manifest:  connectors.Manifest{ID: "fake"},
		items:     []connectors.Item{{ID: "1", Title: "One"}},
		detail:    connectors.Detail{Markdown: &connectors.MarkdownDetail{Content: "detail"}},
	}

	ctx := context.Background()
	assert.Equal(t, "fake", fc.Name())
	assert.True(t, fc.Available(ctx))

	manifest, err := fc.Initialize(ctx)
	require.NoError(t, err)
	assert.Equal(t, "fake", manifest.ID)

	result, err := fc.Search(ctx, connectors.SearchParams{})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, "One", result.Items[0].Title)

	detail, err := fc.FetchDetail(ctx, connectors.FetchDetailParams{ID: "1"})
	require.NoError(t, err)
	assert.Equal(t, "detail", detail.Markdown.Content)
}
