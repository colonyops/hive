package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/colonyops/hive/cmd/desktop/internal/devproxy"
)

// DefaultListen is the devserver's default bind address. Like the desktop's
// webhook listener it binds loopback only: it fronts a GitHub token and can
// mutate what a dev instance sees, neither of which belongs on a LAN.
const DefaultListen = devproxy.DefaultListen

// DefaultUpstream is the GitHub API the proxy fronts. Both REST and GraphQL
// live under it, matching the desktop client's single apiBase.
const DefaultUpstream = "https://api.github.com"

// DefaultTTL is how long a cached response is served without revalidating.
// The desktop's poll floor is 60s (settings.MinPollInterval), so anything at or
// above it collapses every instance's tick onto one upstream call.
const DefaultTTL = 5 * time.Minute

// Config is the parsed devserver.yaml. Every field is optional; the zero value
// is a working passthrough+cache proxy with no fake data.
type Config struct {
	// Listen is the proxy's bind address.
	Listen string `yaml:"listen,omitempty"`
	// Upstream is the GitHub API base the proxy forwards to.
	Upstream string `yaml:"upstream,omitempty"`
	// Cache configures the on-disk response cache.
	Cache CacheConfig `yaml:"cache,omitempty"`
	// Overlays seed the runtime overlay store at startup. The control API and
	// dashboard mutate that store; they never write back here, so a restart
	// returns to exactly what this file declares.
	Overlays []Overlay `yaml:"overlays,omitempty"`
	// Webhooks configures the outbound pusher.
	Webhooks WebhookConfig `yaml:"webhooks,omitempty"`
}

// CacheConfig configures the SQLite response cache.
type CacheConfig struct {
	// Path is the SQLite file. Empty uses DefaultCachePath.
	Path string `yaml:"path,omitempty"`
	// TTL is how long an entry is served before revalidating upstream. A
	// revalidation that returns 304 costs no primary rate-limit quota, so a
	// short TTL is cheap once ETags are stored.
	TTL time.Duration `yaml:"ttl,omitempty"`
}

// Matcher selects which upstream item an overlay applies to. Both fields are
// required: an overlay that matched a whole repo would silently rewrite items
// the author never looked at.
type Matcher struct {
	Repo string `yaml:"repo"`
	Num  int    `yaml:"num"`
}

// Key is the canonical overlay-store key for the matched item.
func (m Matcher) Key() string { return fmt.Sprintf("%s#%d", m.Repo, m.Num) }

// Valid reports whether the matcher identifies exactly one item.
func (m Matcher) Valid() bool {
	return validRepo(m.Repo) && m.Num > 0
}

func validRepo(repo string) bool {
	return strings.Count(repo, "/") == 1 && !strings.HasPrefix(repo, "/") && !strings.HasSuffix(repo, "/")
}

// Overlay is one declared mutation: which item, and what to change about it.
type Overlay struct {
	Match Matcher   `yaml:"match"`
	Set   Mutations `yaml:"set"`
}

// Mutations are the fields an overlay can change. Every field is a pointer or
// nil-able so "not mentioned" stays distinguishable from "set to the zero
// value" — clearing labels and leaving labels alone are different intents.
//
// The set is deliberately narrow: it is exactly what the desktop's GitHub
// classifier reads (cmd/desktop/internal/app/sources/github/classify.go), plus the
// display fields needed to keep a mutated item legible in the feed.
type Mutations struct {
	// State is open, closed, or merged. It drives the classifier's lifecycle
	// transitions and is rendered per response shape — GraphQL wants MERGED,
	// the REST pulls endpoint wants state=closed with merged=true.
	State *string `json:"state,omitempty" yaml:"state,omitempty"`
	// Labels replaces the item's label set.
	Labels *[]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	// Reason is a GitHub notification reason (approval_requested,
	// review_requested, comment, ci_activity, ...). It only appears on
	// notification payloads and is what the classifier turns into an activity
	// summary.
	Reason *string `json:"reason,omitempty" yaml:"reason,omitempty"`
	// Title and Body override the item's display text.
	Title *string `json:"title,omitempty" yaml:"title,omitempty"`
	Body  *string `json:"body,omitempty"  yaml:"body,omitempty"`
	// Draft marks a pull request as a draft.
	Draft *bool `json:"draft,omitempty" yaml:"draft,omitempty"`
	// Absent removes the item from search results while leaving the batched
	// GraphQL state lookup (repository.issueOrPullRequest) answering. That is
	// how GitHub itself behaves once a PR merges out of an is:open query, and
	// it is the only way to exercise the desktop's ConfirmAbsence path.
	Absent *bool `json:"absent,omitempty" yaml:"absent,omitempty"`
	// UpdatedAt overrides the item's update timestamp. Left nil, any applied
	// overlay bumps it to now, because the classifier ignores changes that do
	// not advance updatedAt.
	UpdatedAt *time.Time `json:"updatedAt,omitempty" yaml:"updated_at,omitempty"`
}

// Empty reports whether the mutation set changes nothing.
func (m Mutations) Empty() bool {
	return m.State == nil && m.Labels == nil && m.Reason == nil && m.Title == nil &&
		m.Body == nil && m.Draft == nil && m.Absent == nil && m.UpdatedAt == nil
}

// Merge returns m with every field next explicitly sets overridden. It is how
// successive control-API calls and scenario steps accumulate onto one item
// rather than replacing each other.
func (m Mutations) Merge(next Mutations) Mutations {
	if next.State != nil {
		m.State = next.State
	}
	if next.Labels != nil {
		m.Labels = next.Labels
	}
	if next.Reason != nil {
		m.Reason = next.Reason
	}
	if next.Title != nil {
		m.Title = next.Title
	}
	if next.Body != nil {
		m.Body = next.Body
	}
	if next.Draft != nil {
		m.Draft = next.Draft
	}
	if next.Absent != nil {
		m.Absent = next.Absent
	}
	if next.UpdatedAt != nil {
		m.UpdatedAt = next.UpdatedAt
	}
	return m
}

// validStates are the state values an overlay may set. GitHub models a merged
// PR as closed+merged rather than a distinct state; "merged" is accepted here
// as the author-facing spelling and rendered per shape at apply time.
var validStates = map[string]bool{"open": true, "closed": true, "merged": true}

// WebhookConfig configures the outbound pusher: where it can send, and the
// named bodies it can send.
//
// A payload is an arbitrary JSON object. The desktop's webhook listener keys
// items on a top-level "id" and promotes "title"/"url" for display, so a
// payload wanting to render as a real feed item follows the canonical item
// contract (docs/decisions/0008) — but nothing here enforces that, matching
// the listener's own posture that shape warnings are advisory.
type WebhookConfig struct {
	Targets  []WebhookTarget           `yaml:"targets,omitempty"`
	Payloads map[string]map[string]any `yaml:"payloads,omitempty"`
}

// WebhookTarget is one endpoint the pusher can POST to — in practice a desktop
// instance's local webhook listener, whose base URL the app shows under
// Settings and whose path comes from a sources.webhook node.
type WebhookTarget struct {
	Name string `json:"name" yaml:"name"`
	URL  string `json:"url"  yaml:"url"`
	// Secret is sent as X-Hive-Secret. It must match the sources.webhook node's
	// configured secret; empty means the node accepts unauthenticated pushes.
	//
	// json:"-" makes it unserializable by construction, so no future control-API
	// response can leak it even if it forgets to blank the field first.
	Secret string `json:"-" yaml:"secret,omitempty"`
}

// LoadConfig reads and validates devserver.yaml. A missing file is not an
// error: devserver runs as a plain caching proxy with no config at all, which
// is the whole point for the "just stop burning my rate limit" case.
func LoadConfig(path string) (Config, error) {
	var cfg Config
	data, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err):
		// Fall through to defaults.
	case err != nil:
		return Config{}, fmt.Errorf("read %s: %w", path, err)
	default:
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return Config{}, fmt.Errorf("parse %s: %w", path, err)
		}
	}
	if err := cfg.normalize(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// ResolveConfigPath returns the config file to load: an explicit --config, or
// the checked-in repo config.
//
// There is no per-user config location. Development configuration belongs in
// the repository next to the code it simulates — it is reviewed, versioned,
// and identical for everyone — so a devserver.yaml in a home directory would
// be invisible state that silently changes what a dev instance sees.
//
// An explicit path that does not exist is an error: a typo in a path the
// author typed must not silently degrade to a bare proxy. The default is
// allowed to be missing, which LoadConfig turns into built-in defaults, so
// running the binary from outside a checkout still yields a working proxy.
func ResolveConfigPath(explicit string) (string, error) {
	if explicit == "" {
		return devproxy.RepoConfigPath, nil
	}
	if _, err := os.Stat(explicit); err != nil {
		return "", fmt.Errorf("read %s: %w", explicit, err)
	}
	return explicit, nil
}

// normalize fills defaults and rejects configuration that cannot work. It is
// strict about overlays — a typo there produces a silently wrong simulation,
// which is worse than a startup failure.
func (c *Config) normalize() error {
	if c.Listen == "" {
		c.Listen = DefaultListen
	}
	if c.Upstream == "" {
		c.Upstream = DefaultUpstream
	}
	c.Upstream = strings.TrimSuffix(c.Upstream, "/")
	if c.Cache.TTL <= 0 {
		c.Cache.TTL = DefaultTTL
	}
	if c.Cache.Path == "" {
		c.Cache.Path = DefaultCachePath()
	}

	for i, overlay := range c.Overlays {
		if !overlay.Match.Valid() {
			return fmt.Errorf("overlays[%d]: match needs repo \"owner/name\" and a positive num", i)
		}
		if err := validateMutations(overlay.Set); err != nil {
			return fmt.Errorf("overlays[%d]: %w", i, err)
		}
	}

	seen := make(map[string]bool, len(c.Webhooks.Targets))
	for i, target := range c.Webhooks.Targets {
		if target.Name == "" {
			return fmt.Errorf("webhooks.targets[%d]: name is required", i)
		}
		if seen[target.Name] {
			return fmt.Errorf("webhooks.targets[%d]: duplicate name %q", i, target.Name)
		}
		seen[target.Name] = true
		if !strings.HasPrefix(target.URL, "http://") && !strings.HasPrefix(target.URL, "https://") {
			return fmt.Errorf("webhooks.targets[%d]: url must be http:// or https://", i)
		}
	}
	return nil
}

func validateMutations(m Mutations) error {
	if m.State != nil && !validStates[*m.State] {
		return fmt.Errorf("state %q is not one of open, closed, merged", *m.State)
	}
	return nil
}

// The checked-in config path and the default bind address live in
// cmd/desktop/internal/devproxy: cmd/desktop/devtools reads both to point a worktree's
// launch.env at the same proxy, and a second copy here would let them drift.

// DefaultCachePath is the cache database's default location:
// $XDG_CACHE_HOME/hive/devserver/cache.db, falling back to ~/.cache.
//
// The cache dir, not the data dir, and deliberately blind to the desktop's
// own data root. That root is the *app's* state, and ADR desktop-configuration gives every
// worktree its own isolated copy that `dev:fresh` and `dev:reset` exist to
// delete. Deriving from it would put the cache
// somewhere different for every worktree and throw it away on reset —
// defeating the two things this cache exists to do, which are to be shared
// across instances and to survive restarts.
//
// XDG_CACHE_HOME is also the honest semantic: this is regenerable data that
// costs an API call to rebuild, and nothing breaks if it is deleted.
func DefaultCachePath() string {
	cacheHome := os.Getenv("XDG_CACHE_HOME")
	if cacheHome == "" {
		home, _ := os.UserHomeDir()
		cacheHome = filepath.Join(home, ".cache")
	}
	return filepath.Join(cacheHome, "hive", "devserver", "cache.db")
}
