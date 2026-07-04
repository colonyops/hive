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

// PickerManifest configures how the TUI picker lays out and searches items.
type PickerManifest struct {
	Layout  LayoutMode
	Columns []Column
	Search  SearchManifest
	// HidePreview collapses the picker to a single full-width pane for
	// sources whose items have no useful detail body (e.g. a PR
	// table). The zero value keeps the two-pane list+preview layout.
	HidePreview bool
}

// LayoutMode selects how the picker lays out each item: a single-line
// list, a multi-column table, or a two-line card (title on its own line
// with a status strip beneath it).
type LayoutMode string

// Picker layout modes.
const (
	LayoutModeList  LayoutMode = "list"
	LayoutModeTable LayoutMode = "table"
	LayoutModeCard  LayoutMode = "card"
)

// Column describes one table column when Layout is LayoutModeTable.
type Column struct {
	Key   string
	Label string
	Width int
	Flex  int
}

// SearchManifest configures how the picker issues search queries.
type SearchManifest struct {
	Mode       SearchMode
	DebounceMS int
}

// SearchMode selects between filtering already-loaded items locally and
// issuing a remote Search call per query change.
type SearchMode string

// Picker search modes.
const (
	SearchModeLocal  SearchMode = "local"
	SearchModeRemote SearchMode = "remote"
)

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
