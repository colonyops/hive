// Package cliengine is the shared execution engine behind hive's CLI-backed
// sources (GitHub via gh, Gitea/Forgejo via tea).
//
// Each source is a Driver: static identity/picker properties plus the argv
// construction and JSON parsing behind Search (and, for DetailDrivers,
// FetchDetail). The engine executes drivers — shelling out to the driver's
// binary, caching search output, and validating scope/ID inputs — so adding a
// new CLI-backed source means writing a new Driver, not a new engine.
package cliengine

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

// Config declares a driver's static identity and the CLI binary that services
// it.
type Config struct {
	// ID is the source's registry id and config key (e.g. "issues").
	ID string
	// DisplayName is the picker's tab title.
	DisplayName string
	// Binary is the CLI executable the engine shells out to (e.g. "gh" or
	// "tea").
	Binary string
}

// Driver defines one CLI-backed source.
type Driver interface {
	// Config returns the driver's static identity, binary, and picker layout.
	Config() Config
	// ListArgs builds the argv (without the leading binary) for a Search
	// against scope ("owner/name"), optionally filtered by query.
	ListArgs(scope, query string, limit int) []string
	// ParseList maps the CLI's JSON stdout into source items.
	ParseList(out []byte) ([]sources.Item, error)
}

// DetailDriver is implemented by drivers whose items have a detail view.
type DetailDriver interface {
	Driver
	// DetailArgs builds the argv for a FetchDetail call.
	DetailArgs(scope, id string) []string
	// ParseDetail maps the CLI's JSON stdout into a Detail.
	ParseDetail(out []byte) (sources.Detail, error)
}

// Options tunes the engine. Zero values use package defaults.
type Options struct {
	// SearchLimit caps items per Search call (default 30).
	SearchLimit int
	// CacheTTL bounds how long raw Search output is cached per
	// (scope, dir, query) key (default 30s).
	CacheTTL time.Duration
}

// Source is the shared engine executing a Driver: it shells out to the
// driver's binary via an injected executor, caches raw Search output, and
// converts CLI JSON into source domain types using the driver's parsers.
type Source struct {
	driver Driver
	cfg    Config
	exec   Executor
	cache  *kv.Cache[json.RawMessage]
	limit  int
}

// Executor shells out to the CLI, returning stdout and stderr separately:
// stdout is parsed as JSON and stderr becomes the error message on failure.
// dir is the working directory (empty inherits the process cwd), letting CLIs
// resolve their target host/login from a checkout's git remote.
// *executil.RealExecutor satisfies it via RunOutputDir.
type Executor interface {
	RunOutputDir(ctx context.Context, dir, cmd string, args ...string) (stdout, stderr []byte, err error)
}

var _ sources.Source = (*Source)(nil)

// New constructs a Source executing driver. exec is used to shell out to the
// driver's binary; store backs the search cache and is required.
func New(driver Driver, exec Executor, store kv.KV, opts Options) (*Source, error) {
	cfg := driver.Config()
	if cfg.ID == "" {
		return nil, fmt.Errorf("cliengine driver: id is required")
	}
	if cfg.Binary == "" {
		return nil, fmt.Errorf("cliengine driver %q: binary is required", cfg.ID)
	}
	if store == nil {
		return nil, fmt.Errorf("cliengine driver %q: kv store is required", cfg.ID)
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
		// Cache raw CLI stdout (not parsed items) so cached entries
		// round-trip through JSON storage without mutating Field value
		// types (e.g. int -> float64). Namespaced by binary so the gh and
		// tea backends for the same source id never collide.
		cache: kv.NewCache[json.RawMessage](store, "sources."+cfg.ID+"."+cfg.Binary+".search", ttl),
		limit: limit,
	}, nil
}

// Name returns the source's stable identifier.
func (c *Source) Name() string {
	return c.cfg.ID
}

// Available reports whether the driver's CLI binary is resolvable on PATH.
func (c *Source) Available(_ context.Context) bool {
	_, err := exec.LookPath(c.cfg.Binary)
	return err == nil
}

// Initialize returns the source's picker manifest, derived from the driver's
// Config. Every query re-invokes the CLI, so search is always remote.
func (c *Source) Initialize(_ context.Context) (sources.Manifest, error) {
	_, hasDetail := c.driver.(DetailDriver)
	return sources.Manifest{
		ID:          c.cfg.ID,
		DisplayName: c.cfg.DisplayName,
		Capabilities: sources.Capabilities{
			FetchDetail: hasDetail,
		},
	}, nil
}

// Search returns items in the repository identified by params.Scope
// ("owner/name"), optionally filtered by params.Query. params.Scope is
// required; params.Dir is the working directory the CLI runs in.
func (c *Source) Search(ctx context.Context, params sources.SearchParams) (sources.SearchResult, error) {
	scope, err := parseScope(params.Scope)
	if err != nil {
		return sources.SearchResult{}, fmt.Errorf("%s source: %w", c.cfg.ID, err)
	}

	cacheKey := scope + "|" + params.Dir + "|" + params.Query
	out, cached := c.cache.Get(ctx, cacheKey)
	if !cached {
		args := c.driver.ListArgs(scope, params.Query, c.limit)
		out, err = c.runJSON(ctx, params.Dir, args)
		if err != nil {
			return sources.SearchResult{}, err
		}
		c.cache.Set(ctx, cacheKey, out)
	}

	items, err := c.driver.ParseList(out)
	if err != nil {
		return sources.SearchResult{}, fmt.Errorf("%s source: decode %s output: %w", c.cfg.ID, c.cfg.Binary, err)
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
	// positional argument to the CLI, so a crafted ID (e.g. "--web" or "-1")
	// can never be parsed as a flag. All CLI-backed builtins key items by
	// issue/PR number.
	if params.ID == "" {
		return sources.Detail{}, fmt.Errorf("%s source: fetchDetail requires an id", c.cfg.ID)
	}
	if n, err := strconv.Atoi(params.ID); err != nil || n <= 0 {
		return sources.Detail{}, fmt.Errorf("%s source: invalid id %q: expected a positive number", c.cfg.ID, params.ID)
	}

	out, err := c.runJSON(ctx, params.Dir, detailDriver.DetailArgs(scope, params.ID))
	if err != nil {
		return sources.Detail{}, err
	}

	detail, err := detailDriver.ParseDetail(out)
	if err != nil {
		return sources.Detail{}, fmt.Errorf("%s source: decode %s output: %w", c.cfg.ID, c.cfg.Binary, err)
	}
	return detail, nil
}

func (c *Source) runJSON(ctx context.Context, dir string, args []string) ([]byte, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("%s source: %s: missing arguments", c.cfg.ID, c.cfg.Binary)
	}
	stdout, stderr, err := c.exec.RunOutputDir(ctx, dir, c.cfg.Binary, args...)
	if err != nil {
		if msg := strings.TrimSpace(string(stderr)); msg != "" {
			return nil, fmt.Errorf("%s source: %s %s: %w: %s", c.cfg.ID, c.cfg.Binary, args[0], err, msg)
		}
		return nil, fmt.Errorf("%s source: %s %s: %w", c.cfg.ID, c.cfg.Binary, args[0], err)
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
