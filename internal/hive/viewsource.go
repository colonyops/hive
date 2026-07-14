package hive

import (
	"context"

	"github.com/colonyops/hive/internal/sources"
)

// viewSource wraps a source with a saved query and scope. Every search uses
// the saved values, regardless of caller-supplied query or scope.
type viewSource struct {
	inner       sources.Source
	displayName string
	query       string
	scope       string
}

var _ sources.Source = (*viewSource)(nil)

// Name returns the view's stable identifier.
func (v *viewSource) Name() string {
	return v.inner.Name()
}

// Available passes dependency availability through to the wrapped source.
func (v *viewSource) Available(ctx context.Context) bool {
	return v.inner.Available(ctx)
}

// Initialize preserves the wrapped source's picker details and capabilities
// while exposing the view's identity and display name.
func (v *viewSource) Initialize(ctx context.Context) (sources.Manifest, error) {
	manifest, err := v.inner.Initialize(ctx)
	if err != nil {
		return sources.Manifest{}, err
	}
	manifest.ID = v.Name()
	manifest.DisplayName = v.displayName
	return manifest, nil
}

// Search replaces the caller's query and scope with the view preset.
func (v *viewSource) Search(ctx context.Context, params sources.SearchParams) (sources.SearchResult, error) {
	params.Query = v.query
	params.Scope = v.scope
	return v.inner.Search(ctx, params)
}

// FetchDetail passes item detail requests through unchanged.
func (v *viewSource) FetchDetail(ctx context.Context, params sources.FetchDetailParams) (sources.Detail, error) {
	return v.inner.FetchDetail(ctx, params)
}
