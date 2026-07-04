// Package ghcli implements hive's built-in sources backed by the gh CLI.
//
// Each built-in source is declared as a Spec — a small, mostly
// declarative struct describing its picker manifest, the gh command to run,
// and how to parse gh's JSON output — and executed by the shared Source
// engine in this file. This mirrors how hive plugins are structured: adding
// a new gh-backed source means writing a new Spec (see issues.go and
// prs.go), not a new source implementation.
package ghcli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/colonyops/hive/internal/core/kv"
	"github.com/colonyops/hive/internal/sources"
	"github.com/colonyops/hive/pkg/executil"
)

// defaultSearchLimit bounds how many items a single Search call returns
// when a Spec does not set its own limit.
const defaultSearchLimit = 30

// Spec declares one gh-CLI-backed source: its identity, picker layout,
// and how to build and parse the gh invocations behind Search and
// FetchDetail. Detail support is optional — a Spec with a nil DetailArgs
// advertises no fetchDetail capability and the picker renders accordingly.
type Spec struct {
	// ID is the source's registry id and config key (e.g. "issues").
	ID string
	// DisplayName is the picker's modal title.
	DisplayName string
	// Layout selects the left-pane rendering (list cards vs table rows).
	Layout sources.LayoutMode
	// Columns describes the table columns when Layout is table.
	Columns []sources.Column
	// HidePreview collapses the picker to a single full-width pane for
	// sources whose items have no useful detail body.
	HidePreview bool
	// SearchLimit caps Search results; 0 means defaultSearchLimit.
	SearchLimit int

	// ListArgs builds the gh argv (without the leading "gh") for a Search
	// call against scope ("owner/name"), optionally filtered by query.
	ListArgs func(scope, query string, limit int) []string
	// ParseList maps gh's JSON stdout into source items.
	ParseList func(out []byte) ([]sources.Item, error)

	// DetailArgs builds the gh argv for a FetchDetail call, or nil when
	// the source has no detail view.
	DetailArgs func(scope, id string) []string
	// ParseDetail maps gh's JSON stdout into a Detail. Required when
	// DetailArgs is set.
	ParseDetail func(out []byte) (sources.Detail, error)
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

// Source is the shared engine executing a Spec: it shells out to gh via
// an injected executor, optionally caches raw Search output, and converts
// gh JSON into source domain types using the Spec's parsers.
type Source struct {
	spec  Spec
	exec  executil.Executor
	cache searchCache
}

type outputExecutor interface {
	RunOutput(ctx context.Context, cmd string, args ...string) (stdout, stderr []byte, err error)
}

var _ sources.Source = (*Source)(nil)

// New constructs a Source for spec. exec is used to shell out to gh;
// store may be nil to disable Search result caching.
func New(spec Spec, exec executil.Executor, store kv.KV) (*Source, error) {
	if err := spec.validate(); err != nil {
		return nil, err
	}
	return &Source{
		spec: spec,
		exec: exec,
		// Cache raw gh stdout (not parsed items) so cached entries
		// round-trip through JSON storage without mutating Field value
		// types (e.g. int -> float64).
		cache: newSearchCache(store, "sources."+spec.ID+".search"),
	}, nil
}

// Name returns the source's stable identifier.
func (c *Source) Name() string {
	return c.spec.ID
}

// Available reports whether the gh CLI is resolvable on PATH.
func (c *Source) Available(_ context.Context) bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

// Initialize returns the source's picker manifest, derived entirely from
// the Spec. Search is always remote: every query re-invokes gh.
func (c *Source) Initialize(_ context.Context) (sources.Manifest, error) {
	return sources.Manifest{
		ID:          c.spec.ID,
		DisplayName: c.spec.DisplayName,
		Capabilities: sources.Capabilities{
			FetchDetail: c.spec.DetailArgs != nil,
		},
		Picker: sources.PickerManifest{
			Layout:      c.spec.Layout,
			Columns:     c.spec.Columns,
			HidePreview: c.spec.HidePreview,
			Search: sources.SearchManifest{
				Mode: sources.SearchModeRemote,
			},
		},
	}, nil
}

// Search returns items in the repository identified by params.Scope
// ("owner/name"), optionally filtered by params.Query. params.Scope is
// required.
func (c *Source) Search(ctx context.Context, params sources.SearchParams) (sources.SearchResult, error) {
	scope, err := parseScope(params.Scope)
	if err != nil {
		return sources.SearchResult{}, fmt.Errorf("%s source: %w", c.spec.ID, err)
	}

	cacheKey := scope + "|" + params.Query
	out, cached := c.cache.get(ctx, cacheKey)
	if !cached {
		limit := c.spec.SearchLimit
		if limit <= 0 {
			limit = defaultSearchLimit
		}
		args := c.spec.ListArgs(scope, params.Query, limit)
		out, err = c.runGHJSON(ctx, args)
		if err != nil {
			return sources.SearchResult{}, err
		}
		c.cache.set(ctx, cacheKey, out)
	}

	items, err := c.spec.ParseList(out)
	if err != nil {
		return sources.SearchResult{}, fmt.Errorf("%s source: decode gh output: %w", c.spec.ID, err)
	}
	return sources.SearchResult{Items: items}, nil
}

// FetchDetail returns the detail view for a single item. params.Scope
// ("owner/name") and params.ID (a bare item number) are required.
func (c *Source) FetchDetail(ctx context.Context, params sources.FetchDetailParams) (sources.Detail, error) {
	if c.spec.DetailArgs == nil {
		return sources.Detail{}, fmt.Errorf("%s source: fetchDetail is not supported", c.spec.ID)
	}

	scope, err := parseScope(params.Scope)
	if err != nil {
		return sources.Detail{}, fmt.Errorf("%s source: %w", c.spec.ID, err)
	}

	// Validate the ID is a bare positive number before passing it as a
	// positional argument to gh, so a crafted ID (e.g. "--web" or "-1")
	// can never be parsed as a flag. All gh-backed builtins key items by
	// issue/PR number.
	if params.ID == "" {
		return sources.Detail{}, fmt.Errorf("%s source: fetchDetail requires an id", c.spec.ID)
	}
	if n, err := strconv.Atoi(params.ID); err != nil || n <= 0 {
		return sources.Detail{}, fmt.Errorf("%s source: invalid id %q: expected a positive number", c.spec.ID, params.ID)
	}

	out, err := c.runGHJSON(ctx, c.spec.DetailArgs(scope, params.ID))
	if err != nil {
		return sources.Detail{}, err
	}

	detail, err := c.spec.ParseDetail(out)
	if err != nil {
		return sources.Detail{}, fmt.Errorf("%s source: decode gh output: %w", c.spec.ID, err)
	}
	if !detail.Valid() {
		return sources.Detail{}, fmt.Errorf("%s source: detail has both markdown and kv variants set", c.spec.ID)
	}
	return detail, nil
}

func (c *Source) runGHJSON(ctx context.Context, args []string) ([]byte, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("%s source: gh: missing arguments", c.spec.ID)
	}
	if exec, ok := c.exec.(outputExecutor); ok {
		stdout, stderr, err := exec.RunOutput(ctx, "gh", args...)
		if err != nil {
			msg := strings.TrimSpace(string(stderr))
			if msg != "" {
				return nil, fmt.Errorf("%s source: gh %s: %w: %s", c.spec.ID, args[0], err, msg)
			}
			return nil, fmt.Errorf("%s source: gh %s: %w", c.spec.ID, args[0], err)
		}
		return stdout, nil
	}

	out, err := c.exec.Run(ctx, "gh", args...)
	if err != nil {
		return nil, fmt.Errorf("%s source: gh %s: %w", c.spec.ID, args[0], err)
	}
	return bytes.TrimSpace(out), nil
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
