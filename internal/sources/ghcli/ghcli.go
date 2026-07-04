// Package ghcli implements hive's built-in sources backed by the gh CLI.
//
// Each built-in source is a Driver: static identity/picker properties
// plus the gh argv construction and JSON parsing behind Search (and,
// for DetailDrivers, FetchDetail). The shared Source engine executes
// drivers — shelling out to gh, caching search output, and validating
// scope/ID inputs — so adding a new gh-backed source means writing a new
// driver (see issues.go and prs.go), not a new source implementation.
package ghcli

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/colonyops/hive/internal/core/kv"
	"github.com/colonyops/hive/internal/sources"
)

// Defaults for Options zero values.
const (
	defaultSearchLimit = 30
	defaultCacheTTL    = 30 * time.Second
)

// Config declares a driver's static identity and picker layout.
type Config struct {
	// ID is the source's registry id and config key (e.g. "issues").
	ID string
	// DisplayName is the picker's tab title.
	DisplayName string
	// Layout selects the row rendering (list cards vs table rows).
	Layout sources.LayoutMode
	// Columns describes the table columns when Layout is table.
	Columns []sources.Column
	// HidePreview collapses the picker to a single full-width pane.
	HidePreview bool
}

// Driver defines one gh-CLI-backed source.
type Driver interface {
	// Config returns the driver's static identity and picker layout.
	Config() Config
	// ListArgs builds the gh argv (without the leading "gh") for a
	// Search against scope ("owner/name"), optionally filtered by query.
	ListArgs(scope, query string, limit int) []string
	// ParseList maps gh's JSON stdout into source items.
	ParseList(out []byte) ([]sources.Item, error)
}

// DetailDriver is implemented by drivers whose items have a detail view.
type DetailDriver interface {
	Driver
	// DetailArgs builds the gh argv for a FetchDetail call.
	DetailArgs(scope, id string) []string
	// ParseDetail maps gh's JSON stdout into a Detail.
	ParseDetail(out []byte) (sources.Detail, error)
}

// Options tunes the engine. Zero values use package defaults.
type Options struct {
	// SearchLimit caps items per Search call (default 30).
	SearchLimit int
	// CacheTTL bounds how long raw Search output is cached per
	// (scope, query) key (default 30s).
	CacheTTL time.Duration
}

// Source is the shared engine executing a Driver: it shells out to gh
// via an injected executor, caches raw Search output, and converts gh
// JSON into source domain types using the driver's parsers.
type Source struct {
	driver Driver
	cfg    Config
	exec   Executor
	cache  *kv.Cache[json.RawMessage]
	limit  int
}

// Executor shells out to gh, returning stdout and stderr separately: stdout
// is parsed as JSON and stderr becomes the error message on failure.
// *executil.RealExecutor satisfies it.
type Executor interface {
	RunOutput(ctx context.Context, cmd string, args ...string) (stdout, stderr []byte, err error)
}

var _ sources.Source = (*Source)(nil)

// New constructs a Source executing driver. exec is used to shell out to
// gh; store backs the search cache and is required.
func New(driver Driver, exec Executor, store kv.KV, opts Options) (*Source, error) {
	cfg := driver.Config()
	if cfg.ID == "" {
		return nil, fmt.Errorf("ghcli driver: id is required")
	}
	if store == nil {
		return nil, fmt.Errorf("ghcli driver %q: kv store is required", cfg.ID)
	}

	limit := opts.SearchLimit
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	ttl := opts.CacheTTL
	if ttl <= 0 {
		ttl = defaultCacheTTL
	}

	return &Source{
		driver: driver,
		cfg:    cfg,
		exec:   exec,
		// Cache raw gh stdout (not parsed items) so cached entries
		// round-trip through JSON storage without mutating Field value
		// types (e.g. int -> float64).
		cache: kv.NewCache[json.RawMessage](store, "sources."+cfg.ID+".search", ttl),
		limit: limit,
	}, nil
}

// Name returns the source's stable identifier.
func (c *Source) Name() string {
	return c.cfg.ID
}

// Available reports whether the gh CLI is resolvable on PATH.
func (c *Source) Available(_ context.Context) bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

// Initialize returns the source's picker manifest, derived entirely from
// the driver's Config. Search is always remote: every query re-invokes gh.
func (c *Source) Initialize(_ context.Context) (sources.Manifest, error) {
	_, hasDetail := c.driver.(DetailDriver)
	return sources.Manifest{
		ID:          c.cfg.ID,
		DisplayName: c.cfg.DisplayName,
		Capabilities: sources.Capabilities{
			FetchDetail: hasDetail,
		},
		Picker: sources.PickerManifest{
			Layout:      c.cfg.Layout,
			Columns:     c.cfg.Columns,
			HidePreview: c.cfg.HidePreview,
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
		return sources.SearchResult{}, fmt.Errorf("%s source: %w", c.cfg.ID, err)
	}

	cacheKey := scope + "|" + params.Query
	out, cached := c.cache.Get(ctx, cacheKey)
	if !cached {
		args := c.driver.ListArgs(scope, params.Query, c.limit)
		out, err = c.runGHJSON(ctx, args)
		if err != nil {
			return sources.SearchResult{}, err
		}
		c.cache.Set(ctx, cacheKey, out)
	}

	items, err := c.driver.ParseList(out)
	if err != nil {
		return sources.SearchResult{}, fmt.Errorf("%s source: decode gh output: %w", c.cfg.ID, err)
	}
	return sources.SearchResult{Items: items}, nil
}

// FetchDetail returns the detail view for a single item. params.Scope
// ("owner/name") and params.ID (a bare item number) are required.
func (c *Source) FetchDetail(ctx context.Context, params sources.FetchDetailParams) (sources.Detail, error) {
	detailDriver, ok := c.driver.(DetailDriver)
	if !ok {
		return sources.Detail{}, fmt.Errorf("%s source: fetchDetail is not supported", c.cfg.ID)
	}

	scope, err := parseScope(params.Scope)
	if err != nil {
		return sources.Detail{}, fmt.Errorf("%s source: %w", c.cfg.ID, err)
	}

	// Validate the ID is a bare positive number before passing it as a
	// positional argument to gh, so a crafted ID (e.g. "--web" or "-1")
	// can never be parsed as a flag. All gh-backed builtins key items by
	// issue/PR number.
	if params.ID == "" {
		return sources.Detail{}, fmt.Errorf("%s source: fetchDetail requires an id", c.cfg.ID)
	}
	if n, err := strconv.Atoi(params.ID); err != nil || n <= 0 {
		return sources.Detail{}, fmt.Errorf("%s source: invalid id %q: expected a positive number", c.cfg.ID, params.ID)
	}

	out, err := c.runGHJSON(ctx, detailDriver.DetailArgs(scope, params.ID))
	if err != nil {
		return sources.Detail{}, err
	}

	detail, err := detailDriver.ParseDetail(out)
	if err != nil {
		return sources.Detail{}, fmt.Errorf("%s source: decode gh output: %w", c.cfg.ID, err)
	}
	return detail, nil
}

func (c *Source) runGHJSON(ctx context.Context, args []string) ([]byte, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("%s source: gh: missing arguments", c.cfg.ID)
	}
	stdout, stderr, err := c.exec.RunOutput(ctx, "gh", args...)
	if err != nil {
		if msg := strings.TrimSpace(string(stderr)); msg != "" {
			return nil, fmt.Errorf("%s source: gh %s: %w: %s", c.cfg.ID, args[0], err, msg)
		}
		return nil, fmt.Errorf("%s source: gh %s: %w", c.cfg.ID, args[0], err)
	}
	return stdout, nil
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

// ghRef is a minimal issue/PR cross-reference: the PRs that would close an
// issue (closedByPullRequestsReferences) or the issues a PR closes
// (closingIssuesReferences).
type ghRef struct {
	Number int `json:"number"`
}

// assigneeSummary returns the first assignee login and the total count.
func assigneeSummary(assignees []ghAuthor) (login string, count int) {
	if len(assignees) == 0 {
		return "", 0
	}
	return assignees[0].Login, len(assignees)
}

// firstRef returns the first cross-reference's number and the total count.
// A zero number means there are no references.
func firstRef(refs []ghRef) (number, count int) {
	if len(refs) == 0 {
		return 0, 0
	}
	return refs[0].Number, len(refs)
}

// timeNow is overridable in tests so age formatting is deterministic.
var timeNow = time.Now

// shortAge renders a compact age like "3w", "5d", "2mo", or "1y" relative
// to timeNow. It returns "" for a zero or future timestamp.
func shortAge(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := timeNow().Sub(t)
	switch {
	case d < 0:
		return ""
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dw", int(d.Hours()/(24*7)))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo", int(d.Hours()/(24*30)))
	default:
		return fmt.Sprintf("%dy", int(d.Hours()/(24*365)))
	}
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
