// Package ghcli implements hive's built-in connectors backed by the gh CLI.
//
// Each built-in connector is declared as a Spec — a small, mostly
// declarative struct describing its picker manifest, the gh command to run,
// and how to parse gh's JSON output — and executed by the shared Connector
// engine in this file. This mirrors how hive plugins are structured: adding
// a new gh-backed connector means writing a new Spec (see issues.go and
// prs.go), not a new connector implementation.
package ghcli

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/colonyops/hive/internal/connectors"
	"github.com/colonyops/hive/internal/core/kv"
	"github.com/colonyops/hive/pkg/executil"
)

// defaultSearchLimit bounds how many items a single Search call returns
// when a Spec does not set its own limit.
const defaultSearchLimit = 30

// Spec declares one gh-CLI-backed connector: its identity, picker layout,
// and how to build and parse the gh invocations behind Search and
// FetchDetail. Detail support is optional — a Spec with a nil DetailArgs
// advertises no fetchDetail capability and the picker renders accordingly.
type Spec struct {
	// ID is the connector's registry id and config key (e.g. "issues").
	ID string
	// DisplayName is the picker's modal title.
	DisplayName string
	// Layout selects the left-pane rendering (list cards vs table rows).
	Layout connectors.LayoutMode
	// Columns describes the table columns when Layout is table.
	Columns []connectors.Column
	// HidePreview collapses the picker to a single full-width pane for
	// connectors whose items have no useful detail body.
	HidePreview bool
	// SearchLimit caps Search results; 0 means defaultSearchLimit.
	SearchLimit int

	// ListArgs builds the gh argv (without the leading "gh") for a Search
	// call against scope ("owner/name"), optionally filtered by query.
	ListArgs func(scope, query string, limit int) []string
	// ParseList maps gh's JSON stdout into connector items.
	ParseList func(out []byte) ([]connectors.Item, error)

	// DetailArgs builds the gh argv for a FetchDetail call, or nil when
	// the connector has no detail view.
	DetailArgs func(scope, id string) []string
	// ParseDetail maps gh's JSON stdout into a Detail. Required when
	// DetailArgs is set.
	ParseDetail func(out []byte) (connectors.Detail, error)
}

// validate reports Spec construction errors early (at wiring time) instead
// of surfacing them as confusing runtime failures.
func (s Spec) validate() error {
	switch {
	case s.ID == "":
		return fmt.Errorf("ghcli spec: id is required")
	case s.ListArgs == nil || s.ParseList == nil:
		return fmt.Errorf("ghcli spec %q: ListArgs and ParseList are required", s.ID)
	case (s.DetailArgs == nil) != (s.ParseDetail == nil):
		return fmt.Errorf("ghcli spec %q: DetailArgs and ParseDetail must be set together", s.ID)
	}
	return nil
}

// Connector is the shared engine executing a Spec: it shells out to gh via
// an injected executor, optionally caches raw Search output, and converts
// gh JSON into connector domain types using the Spec's parsers.
type Connector struct {
	spec  Spec
	exec  executil.Executor
	cache searchCache
}

var _ connectors.Connector = (*Connector)(nil)

// New constructs a Connector for spec. exec is used to shell out to gh;
// store may be nil to disable Search result caching.
func New(spec Spec, exec executil.Executor, store kv.KV) (*Connector, error) {
	if err := spec.validate(); err != nil {
		return nil, err
	}
	return &Connector{
		spec: spec,
		exec: exec,
		// Cache raw gh stdout (not parsed items) so cached entries
		// round-trip through JSON storage without mutating Field value
		// types (e.g. int -> float64).
		cache: newSearchCache(store, "connectors."+spec.ID+".search"),
	}, nil
}

// Name returns the connector's stable identifier.
func (c *Connector) Name() string {
	return c.spec.ID
}

// Available reports whether the gh CLI is resolvable on PATH.
func (c *Connector) Available(_ context.Context) bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

// Initialize returns the connector's picker manifest, derived entirely from
// the Spec. Search is always remote: every query re-invokes gh.
func (c *Connector) Initialize(_ context.Context) (connectors.Manifest, error) {
	return connectors.Manifest{
		ID:          c.spec.ID,
		DisplayName: c.spec.DisplayName,
		Capabilities: connectors.Capabilities{
			FetchDetail: c.spec.DetailArgs != nil,
		},
		Picker: connectors.PickerManifest{
			Layout:      c.spec.Layout,
			Columns:     c.spec.Columns,
			HidePreview: c.spec.HidePreview,
			Search: connectors.SearchManifest{
				Mode: connectors.SearchModeRemote,
			},
		},
	}, nil
}

// Search returns items in the repository identified by params.Scope
// ("owner/name"), optionally filtered by params.Query. params.Scope is
// required.
func (c *Connector) Search(ctx context.Context, params connectors.SearchParams) (connectors.SearchResult, error) {
	scope, err := parseScope(params.Scope)
	if err != nil {
		return connectors.SearchResult{}, fmt.Errorf("%s connector: %w", c.spec.ID, err)
	}

	cacheKey := scope + "|" + params.Query
	out, cached := c.cache.get(ctx, cacheKey)
	if !cached {
		limit := c.spec.SearchLimit
		if limit <= 0 {
			limit = defaultSearchLimit
		}
		args := c.spec.ListArgs(scope, params.Query, limit)
		out, err = c.exec.Run(ctx, "gh", args...)
		if err != nil {
			return connectors.SearchResult{}, fmt.Errorf("%s connector: gh %s: %w", c.spec.ID, args[0], err)
		}
		c.cache.set(ctx, cacheKey, out)
	}

	items, err := c.spec.ParseList(out)
	if err != nil {
		return connectors.SearchResult{}, fmt.Errorf("%s connector: decode gh output: %w", c.spec.ID, err)
	}
	return connectors.SearchResult{Items: items}, nil
}

// FetchDetail returns the detail view for a single item. params.Scope
// ("owner/name") and params.ID (a bare item number) are required.
func (c *Connector) FetchDetail(ctx context.Context, params connectors.FetchDetailParams) (connectors.Detail, error) {
	if c.spec.DetailArgs == nil {
		return connectors.Detail{}, fmt.Errorf("%s connector: fetchDetail is not supported", c.spec.ID)
	}

	scope, err := parseScope(params.Scope)
	if err != nil {
		return connectors.Detail{}, fmt.Errorf("%s connector: %w", c.spec.ID, err)
	}

	// Validate the ID is a bare number before passing it as a positional
	// argument to gh, so a crafted ID (e.g. "--web") can never be parsed
	// as a flag. All gh-backed builtins key items by issue/PR number.
	if params.ID == "" {
		return connectors.Detail{}, fmt.Errorf("%s connector: fetchDetail requires an id", c.spec.ID)
	}
	if _, err := strconv.Atoi(params.ID); err != nil {
		return connectors.Detail{}, fmt.Errorf("%s connector: invalid id %q: expected a number", c.spec.ID, params.ID)
	}

	out, err := c.exec.Run(ctx, "gh", c.spec.DetailArgs(scope, params.ID)...)
	if err != nil {
		return connectors.Detail{}, fmt.Errorf("%s connector: gh: %w", c.spec.ID, err)
	}

	detail, err := c.spec.ParseDetail(out)
	if err != nil {
		return connectors.Detail{}, fmt.Errorf("%s connector: decode gh output: %w", c.spec.ID, err)
	}
	return detail, nil
}

// parseScope validates that scope has the shape "owner/name" and returns it
// normalized.
func parseScope(scope string) (string, error) {
	if scope == "" {
		return "", fmt.Errorf("scope is required (expected owner/name)")
	}
	parts := strings.Split(scope, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", fmt.Errorf("invalid scope %q: expected owner/name", scope)
	}
	return parts[0] + "/" + parts[1], nil
}

// ghAuthor is the author sub-object embedded in gh list JSON.
type ghAuthor struct {
	Login string `json:"login"`
}

// ghLabel is a single label sub-object embedded in gh list JSON.
type ghLabel struct {
	Name string `json:"name"`
}

// labelNames extracts non-empty label names.
func labelNames(labels []ghLabel) []string {
	names := make([]string, 0, len(labels))
	for _, label := range labels {
		if label.Name != "" {
			names = append(names, label.Name)
		}
	}
	return names
}

// decodeList unmarshals gh's JSON array stdout into T entries.
func decodeList[T any](out []byte) ([]T, error) {
	var entries []T
	if err := json.Unmarshal(out, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// decodeJSON unmarshals gh's JSON object stdout into dest.
func decodeJSON(out []byte, dest any) error {
	return json.Unmarshal(out, dest)
}
