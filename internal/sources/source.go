// Package sources defines the domain contract for browsing external
// systems (GitHub issues, and later Gitea/Grafana/etc.) and mapping a
// selected item into hive session-creation inputs.
package sources

import "context"

// Source browses an external system and returns items hive can display
// and map into sessions. Implementations may run in-process (e.g. GitHub via
// the gh CLI).
type Source interface {
	// Name returns the source's stable identifier (e.g. "github").
	Name() string
	// Available reports whether the source's runtime dependencies
	// (binaries, auth, etc.) are satisfied.
	Available(ctx context.Context) bool
	// Initialize returns the source's display/picker manifest.
	Initialize(ctx context.Context) (Manifest, error)
	// Search returns items matching the given query/scope.
	Search(ctx context.Context, params SearchParams) (SearchResult, error)
	// FetchDetail returns the detail view for a single item.
	FetchDetail(ctx context.Context, params FetchDetailParams) (Detail, error)
}

// Manifest describes a source's identity and how the picker should
// display its items.
type Manifest struct {
	ID           string
	DisplayName  string
	Capabilities Capabilities
	Picker       PickerManifest
}

// Capabilities declares optional source behavior.
type Capabilities struct {
	FetchDetail bool
}

// PickerManifest configures how the TUI picker searches a source's items.
type PickerManifest struct {
	Search SearchManifest
}

// SearchManifest configures how the picker issues search queries.
type SearchManifest struct {
	// DebounceMS is the delay before a query change issues a remote search;
	// zero uses the picker's default debounce.
	DebounceMS int
}

// SearchParams carries a search query and scope to a source.
type SearchParams struct {
	Query string
	Scope string
	// Cursor is an opaque pagination cursor; empty for the first page.
	// Remote sources may ignore it.
	Cursor string
}

// SearchResult is the response to a Search call.
type SearchResult struct {
	Items []Item
	// NextCursor is opaque; empty when there are no further pages. This is a
	// seam for future pagination support and may always be left empty.
	NextCursor string
}

// FetchDetailParams carries the scope/URI alongside the ID so detail
// requests are self-contained and do not rely on IDs implicitly encoding
// their repository/org. This keeps the contract general for sources
// added later.
type FetchDetailParams struct {
	ID    string
	Scope string
	// URI is the stable item URI when the source supplies one; optional.
	URI string
}

// Item is a single browsable/selectable record returned by Search.
type Item struct {
	ID       string
	Title    string
	Subtitle string
	// URI is a stable identifier echoed back on FetchDetail; optional.
	URI    string
	Fields map[string]any
}

// Detail is an item's optional detail body, fetched via Source.FetchDetail.
// A nil Markdown means the item has no detail (a PR row, or a fetch that
// failed), so consumers render an empty body rather than panicking.
type Detail struct {
	Markdown *MarkdownDetail
}

// MarkdownDetail renders as markdown via the shared glamour renderer.
type MarkdownDetail struct {
	Content string
}
