package hive

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/sources"
)

type recordingSource struct {
	name         string
	available    bool
	manifest     sources.Manifest
	searchResult sources.SearchResult
	detail       sources.Detail
	searchParams []sources.SearchParams
	detailParams []sources.FetchDetailParams
}

func (s *recordingSource) Name() string {
	return s.name
}

func (s *recordingSource) Available(context.Context) bool {
	return s.available
}

func (s *recordingSource) Initialize(context.Context) (sources.Manifest, error) {
	return s.manifest, nil
}

func (s *recordingSource) Search(_ context.Context, params sources.SearchParams) (sources.SearchResult, error) {
	s.searchParams = append(s.searchParams, params)
	return s.searchResult, nil
}

func (s *recordingSource) FetchDetail(_ context.Context, params sources.FetchDetailParams) (sources.Detail, error) {
	s.detailParams = append(s.detailParams, params)
	return s.detail, nil
}

func TestViewSource(t *testing.T) {
	markdown := &sources.MarkdownDetail{Content: "detail"}
	inner := &recordingSource{
		name:      "review-queue",
		available: true,
		manifest: sources.Manifest{
			ID:          "inner-id",
			DisplayName: "Inner Name",
			Capabilities: sources.Capabilities{
				FetchDetail: true,
			},
			Picker: sources.PickerManifest{
				Search: sources.SearchManifest{DebounceMS: 275},
			},
		},
		searchResult: sources.SearchResult{Items: []sources.Item{{ID: "42"}}, NextCursor: "next"},
		detail:       sources.Detail{Markdown: markdown},
	}
	view := &viewSource{
		inner:       inner,
		displayName: "Review Queue",
		query:       "review-requested:@me",
		scope:       "owner/repo",
	}
	ctx := context.Background()

	assert.Equal(t, "review-queue", view.Name())
	assert.True(t, view.Available(ctx))
	manifest, err := view.Initialize(ctx)
	require.NoError(t, err)
	assert.Equal(t, "review-queue", manifest.ID)
	assert.Equal(t, "Review Queue", manifest.DisplayName)
	assert.Equal(t, inner.manifest.Capabilities, manifest.Capabilities)
	assert.Equal(t, inner.manifest.Picker, manifest.Picker)

	result, err := view.Search(ctx, sources.SearchParams{
		Query:  "caller query",
		Scope:  "caller/scope",
		Dir:    "/repo/path",
		Cursor: "cursor",
	})
	require.NoError(t, err)
	assert.Equal(t, inner.searchResult, result)
	require.Len(t, inner.searchParams, 1)
	assert.Equal(t, sources.SearchParams{
		Query:  "review-requested:@me",
		Scope:  "owner/repo",
		Dir:    "/repo/path",
		Cursor: "cursor",
	}, inner.searchParams[0])

	detailParams := sources.FetchDetailParams{
		ID: "42", Scope: "item/repo", URI: "https://example.test/42", Dir: "/repo/path",
	}
	detail, err := view.FetchDetail(ctx, detailParams)
	require.NoError(t, err)
	assert.Equal(t, markdown, detail.Markdown)
	require.Len(t, inner.detailParams, 1)
	assert.Equal(t, detailParams, inner.detailParams[0])
}
