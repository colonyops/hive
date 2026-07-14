package sources_test

import (
	"context"
	"testing"

	"github.com/colonyops/hive/internal/sources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeSource is a minimal in-memory Source implementation shared by
// tests in this package and later phases (TUI, registry).
type fakeSource struct {
	name      string
	available bool
	manifest  sources.Manifest
	items     []sources.Item
	detail    sources.Detail

	initErr   error
	searchErr error
	detailErr error
}

func (f *fakeSource) Name() string { return f.name }

func (f *fakeSource) Available(_ context.Context) bool { return f.available }

func (f *fakeSource) Initialize(_ context.Context) (sources.Manifest, error) {
	if f.initErr != nil {
		return sources.Manifest{}, f.initErr
	}
	return f.manifest, nil
}

func (f *fakeSource) Search(_ context.Context, _ sources.SearchParams) (sources.SearchResult, error) {
	if f.searchErr != nil {
		return sources.SearchResult{}, f.searchErr
	}
	return sources.SearchResult{Items: f.items}, nil
}

func (f *fakeSource) FetchDetail(_ context.Context, _ sources.FetchDetailParams) (sources.Detail, error) {
	if f.detailErr != nil {
		return sources.Detail{}, f.detailErr
	}
	return f.detail, nil
}

var _ sources.Source = (*fakeSource)(nil)

func TestRenderSessionTemplates(t *testing.T) {
	tests := []struct {
		name       string
		cfg        sources.TemplateConfig
		item       sources.Item
		detail     sources.Detail
		wantName   string
		wantPrompt string
		wantTags   []string
		wantErr    string
	}{
		{
			name: "renders name prompt and tags from fields",
			cfg: sources.TemplateConfig{
				Name:   "gh-{{ .Fields.number }}-{{ .Title }}",
				Prompt: "Work on {{ .Title }}\n\n{{ .Fields.url }}",
				Tags:   []string{"github", "issue-{{ .Fields.number }}"},
			},
			item: sources.Item{
				ID:    "1",
				Title: "Fix bug",
				Fields: map[string]any{
					"number": 42,
					"url":    "https://example.com/1",
				},
			},
			wantName:   "gh-42-fix-bug",
			wantPrompt: "Work on Fix bug\n\nhttps://example.com/1",
			wantTags:   []string{"github", "issue-42"},
		},
		{
			name: "kebab cases punctuation and colons in names",
			cfg: sources.TemplateConfig{
				Name:   "{{ .Title }}",
				Prompt: "ok",
			},
			item: sources.Item{
				ID:    "1",
				Title: "Area: Fix HTTP/2 Bug!",
			},
			wantName:   "area-fix-http-2-bug",
			wantPrompt: "ok",
			wantTags:   []string{},
		},
		{
			name: "truncates long names at a word boundary",
			cfg: sources.TemplateConfig{
				Name:   "gh-{{ .Fields.number }}-{{ .Title }}",
				Prompt: "ok",
			},
			item: sources.Item{
				ID:    "1",
				Title: "Add support for configuring session name truncation behavior when creating sessions from external sources",
				Fields: map[string]any{
					"number": 1234,
				},
			},
			wantName:   "gh-1234-add-support-for-configuring-session-name-truncation",
			wantPrompt: "ok",
			wantTags:   []string{},
		},
		{
			name: "truncates hard at limit when no hyphen boundary exists",
			cfg: sources.TemplateConfig{
				Name:   "{{ .Title }}",
				Prompt: "ok",
			},
			item: sources.Item{
				ID:    "1",
				Title: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			},
			wantName:   "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"[:60],
			wantPrompt: "ok",
			wantTags:   []string{},
		},
		{
			name: "renders detail content",
			cfg: sources.TemplateConfig{
				Name:   "{{ .ID }}",
				Prompt: "{{ .Detail }}",
			},
			item:       sources.Item{ID: "1", Title: "Item"},
			detail:     sources.Detail{Markdown: &sources.MarkdownDetail{Content: "body text"}},
			wantName:   "1",
			wantPrompt: "body text",
			wantTags:   []string{},
		},
		{
			name: "missing field errors on name template",
			cfg: sources.TemplateConfig{
				Name:   "{{ .Fields.missing }}",
				Prompt: "ok",
			},
			item:    sources.Item{ID: "1", Title: "Item", Fields: map[string]any{}},
			wantErr: "name template",
		},
		{
			name: "missing field errors on prompt template",
			cfg: sources.TemplateConfig{
				Name:   "ok",
				Prompt: "{{ .Fields.missing }}",
			},
			item:    sources.Item{ID: "1", Title: "Item", Fields: map[string]any{}},
			wantErr: "prompt template",
		},
		{
			name: "missing field errors on tags template",
			cfg: sources.TemplateConfig{
				Name:   "ok",
				Prompt: "ok",
				Tags:   []string{"{{ .Fields.missing }}"},
			},
			item:    sources.Item{ID: "1", Title: "Item", Fields: map[string]any{}},
			wantErr: "tags[0] template",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sources.RenderSessionTemplates(tt.cfg, tt.item, tt.detail)
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

// Interface compliance of the test double is guaranteed at compile time by
// the `var _ sources.Source = (*fakeSource)(nil)` assertion above;
// no runtime test is needed.
