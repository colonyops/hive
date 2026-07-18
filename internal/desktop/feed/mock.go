package feed

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// MockProvider serves the fixture data the e2e suite snapshots. It is the
// provider in HIVE_DESKTOP_MOCK modes; MarkRead is a no-op and Items are
// static so repeated e2e interactions never mutate the fixtures. Sources,
// profiles, and feed definitions are the mutable pieces: the onboarding
// variant starts empty so e2e can walk workspace creation and the
// create-source → create-feed → edit-feed flow.
type MockProvider struct {
	mu       sync.Mutex
	profiles []Profile
	sources  []SourceDef
	feedDefs map[string]map[string]FeedDef // profileID → feedID → def
}

// NewMockProvider starts with the fixture profile (mock mode "feed").
func NewMockProvider() *MockProvider {
	p := &MockProvider{sources: DefaultSources(), feedDefs: make(map[string]map[string]FeedDef)}
	p.profiles = []Profile{fixtureProfile("hive-core", "Frontend Triage")}
	p.seedFeedDefs("hive-core")
	return p
}

// NewEmptyMockProvider starts with no profiles (mock mode "onboarding"), so
// the workspace-creation step is reachable offline.
func NewEmptyMockProvider() *MockProvider {
	return &MockProvider{feedDefs: make(map[string]map[string]FeedDef)}
}

func (p *MockProvider) seedFeedDefs(profileID string) {
	defs := make(map[string]FeedDef, len(DefaultFeeds()))
	for _, def := range DefaultFeeds() {
		defs[def.ID] = def
	}
	p.feedDefs[profileID] = defs
}

func fixtureProfile(id, name string) Profile {
	return Profile{
		ID:            id,
		Letter:        ProfileLetter(name),
		Name:          name,
		SourceSummary: "GitHub · 3 sources",
		TotalCount:    23,
		UnreadCount:   4,
		Feeds: []Source{
			{ID: "my-open-prs", Name: "My open PRs", Count: 12, NewCount: 0},
			{ID: "notifications-inbox", Name: "Notifications inbox", Count: 3, NewCount: 3},
			{ID: "assigned-across-org", Name: "Assigned across org", Count: 8, NewCount: 0},
		},
	}
}

func (p *MockProvider) Profiles(context.Context) ([]Profile, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]Profile, len(p.profiles))
	copy(out, p.profiles)
	return out, nil
}

func (p *MockProvider) Items(_ context.Context, _, _ string) ([]Item, error) {
	return []Item{
		{
			ID: "pr2841", Kind: "PR", Repo: "hive/core", Num: 2841,
			Title:  "batch_spawn: fix detached tmux env & PATH propagation",
			Author: "lena", Age: "2h", Unread: true, Labels: []string{"bug", "batch"},
			Branch: "fix/2841-batch-spawn-env",
			Body:   "Sessions spawned from a GUI context inherit an empty PATH and lose HIVE_* vars, so batch_spawn fails to find the agent binary. Needs a controlled env when there is no controlling terminal.",
			Prompt: "Investigate detached tmux env in batch_spawn; ensure PATH and HIVE_* propagate when spawned headless from the desktop app.",
			URL:    "https://github.com/hive/core/pull/2841",
		},
		{
			ID: "iss1190", Kind: "Issue", Repo: "hive/desktop", Num: 1190,
			Title:  "Feed source: mirror GitHub notifications inbox",
			Author: "hayden", Age: "5h", Unread: true, Reason: "mention", Labels: []string{"feature", "mvp"},
			Branch: "feat/1190-notifications-feed",
			Body:   "Add a notifications-based feed source that mirrors the user’s GitHub inbox, with local read/dismiss state so triage does not touch GitHub until the user acts.",
			Prompt: "Implement a notifications-based feed source mirroring the GitHub inbox, with app-local read/dismiss triage state.",
			URL:    "https://github.com/hive/desktop/issues/1190",
		},
		{
			ID: "pr2838", Kind: "PR", Repo: "hive/desktop", Num: 2838,
			Title:  "OAuth device flow for in-app GitHub auth",
			Author: "koji", Age: "1d", Unread: false, Labels: []string{"auth"},
			Branch: "feat/2838-oauth-device-flow",
			Body:   "Adds the full device-flow auth so users can sign in without leaving the app. Open question on GitHub App vs OAuth App registration and where to store the token.",
			Prompt: "Review the OAuth device-flow implementation and validate keychain token storage across platforms.",
			URL:    "https://github.com/hive/desktop/pull/2838",
		},
		{
			ID: "iss1204", Kind: "Issue", Repo: "hive/desktop", Num: 1204,
			Title:  "Composable view contract for feed / task / doc surfaces",
			Author: "mira", Age: "1d", Unread: true, Labels: []string{"arch"},
			Branch: "feat/1204-composable-views",
			Body:   "Define a self-contained component contract for feed, task, and doc views so a designer-led layout system can be dropped in later without rewrites.",
			Prompt: "Draft a composable, self-contained view interface covering the feed, task list, and doc viewer surfaces.",
			URL:    "https://github.com/hive/desktop/issues/1204",
		},
		{
			ID: "pr2830", Kind: "PR", Repo: "hive/core", Num: 2830,
			Title:  "Keychain-backed token storage",
			Author: "sam", Age: "2d", Unread: false, Labels: []string{"security"},
			Branch: "feat/2830-keychain-tokens",
			Body:   "Store GitHub tokens in the OS keychain instead of a plaintext config file, with a fallback for headless CI environments.",
			Prompt: "Review cross-platform keychain token storage and the headless fallback path.",
			URL:    "https://github.com/hive/core/pull/2830",
		},
		{
			ID: "iss1177", Kind: "Issue", Repo: "hive/desktop", Num: 1177,
			Title:  "Cross-repo query: PRs assigned to me across the org",
			Author: "hayden", Age: "3d", Unread: false, Labels: []string{"feature"},
			Branch: "feat/1177-cross-repo-query",
			Body:   "Support GitHub search-style cross-repo queries as a feed source, e.g. \"PRs assigned to me across the org\", saveable as a workspace source.",
			Prompt: "Implement a cross-repo query feed source using GitHub search syntax, saveable into a workspace.",
			URL:    "https://github.com/hive/desktop/issues/1177",
		},
	}, nil
}

func (p *MockProvider) ActionsFor(kind string) []Action {
	return ActionsFor(kind)
}

func (p *MockProvider) MarkRead(context.Context, string, string) error {
	return nil
}

// Refresh is a no-op: the fixtures never change.
func (p *MockProvider) Refresh(context.Context, string) (bool, error) {
	return false, nil
}

// Config returns a fixture config so the feeds-as-code sheet is walkable
// offline. The stable path keeps e2e snapshots deterministic.
func (p *MockProvider) Config(context.Context) (ConfigInfo, error) {
	return ConfigInfo{
		Path:   "~/.config/hive/desktop/profiles.yaml",
		Exists: true,
		YAML:   ExampleConfig(),
		Valid:  true,
	}, nil
}

// CreateProfile appends a fixture-backed profile with the given name, seeding
// the default sources and feed definitions like the live store does.
// Deterministic ID: e2e asserts against a stable snapshot.
func (p *MockProvider) CreateProfile(_ context.Context, name string) (Profile, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Profile{}, fmt.Errorf("feed: profile name is empty")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	profile := fixtureProfile(fmt.Sprintf("workspace-%d", len(p.profiles)+1), name)
	p.profiles = append(p.profiles, profile)
	p.seedFeedDefs(profile.ID)
	existing := make(map[string]bool, len(p.sources))
	for _, src := range p.sources {
		existing[src.ID] = true
	}
	for _, src := range DefaultSources() {
		if !existing[src.ID] {
			p.sources = append(p.sources, src)
		}
	}
	return profile, nil
}

// Sources returns the in-memory source definitions.
func (p *MockProvider) Sources(context.Context) ([]SourceDef, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]SourceDef, len(p.sources))
	copy(out, p.sources)
	return out, nil
}

// FeedDefFor returns the stored feed definition, for edit prefill.
func (p *MockProvider) FeedDefFor(_ context.Context, profileID, feedID string) (FeedDef, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	defs, ok := p.feedDefs[profileID]
	if !ok {
		return FeedDef{}, fmt.Errorf("feed: unknown profile %q", profileID)
	}
	def, ok := defs[feedID]
	if !ok {
		return FeedDef{}, fmt.Errorf("feed: unknown feed %q in profile %q", feedID, profileID)
	}
	return def, nil
}

// CreateSource validates and stores a source in memory, uniquifying its ID
// like the live store.
func (p *MockProvider) CreateSource(_ context.Context, def SourceDef) (SourceDef, error) {
	slug := slugify(def.ID)
	if slug == "" {
		return SourceDef{}, fmt.Errorf("feed: source id is empty")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	taken := make(map[string]bool, len(p.sources))
	for _, src := range p.sources {
		taken[src.ID] = true
	}
	def.ID = uniqueID(slug, taken)
	if err := validateSource(def); err != nil {
		return SourceDef{}, fmt.Errorf("feed: %w", err)
	}
	p.sources = append(p.sources, def)
	return def, nil
}

// CreateFeed stores a feed definition and appends its summary to the
// profile's feed list, deriving the ID from the name like the live store.
func (p *MockProvider) CreateFeed(_ context.Context, profileID string, def FeedDef) (Source, error) {
	def.Name = strings.TrimSpace(def.Name)
	if def.Name == "" {
		return Source{}, fmt.Errorf("feed: feed name is empty")
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	at := p.profileIndexLocked(profileID)
	if at < 0 {
		return Source{}, fmt.Errorf("feed: unknown profile %q", profileID)
	}
	if err := p.validateFeedSourcesLocked(def); err != nil {
		return Source{}, err
	}

	slug := slugify(def.Name)
	if slug == "" {
		slug = "feed"
	}
	defs := p.feedDefs[profileID]
	if defs == nil {
		defs = make(map[string]FeedDef)
		p.feedDefs[profileID] = defs
	}
	taken := make(map[string]bool, len(defs))
	for id := range defs {
		taken[id] = true
	}
	def.ID = uniqueID(slug, taken)
	defs[def.ID] = def

	summary := Source{ID: def.ID, Name: def.Name}
	p.profiles[at].Feeds = append(p.profiles[at].Feeds, summary)
	return summary, nil
}

// UpdateFeed replaces the stored feed definition; the feed keeps its ID.
func (p *MockProvider) UpdateFeed(_ context.Context, profileID, feedID string, def FeedDef) error {
	def.ID = feedID
	def.Name = strings.TrimSpace(def.Name)
	if def.Name == "" {
		return fmt.Errorf("feed: feed name is empty")
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	at := p.profileIndexLocked(profileID)
	if at < 0 {
		return fmt.Errorf("feed: unknown profile %q", profileID)
	}
	defs := p.feedDefs[profileID]
	if _, ok := defs[feedID]; !ok {
		return fmt.Errorf("feed: unknown feed %q in profile %q", feedID, profileID)
	}
	if err := p.validateFeedSourcesLocked(def); err != nil {
		return err
	}
	defs[feedID] = def
	for i, summary := range p.profiles[at].Feeds {
		if summary.ID == feedID {
			p.profiles[at].Feeds[i].Name = def.Name
		}
	}
	return nil
}

func (p *MockProvider) profileIndexLocked(profileID string) int {
	for i, profile := range p.profiles {
		if profile.ID == profileID {
			return i
		}
	}
	return -1
}

// validateFeedSourcesLocked mirrors the live config validation: a feed needs
// at least one source and every reference must resolve.
func (p *MockProvider) validateFeedSourcesLocked(def FeedDef) error {
	if len(def.Sources) == 0 {
		return fmt.Errorf("feed: at least one source is required")
	}
	known := make(map[string]bool, len(p.sources))
	for _, src := range p.sources {
		known[src.ID] = true
	}
	for _, sourceID := range def.Sources {
		if !known[sourceID] {
			return fmt.Errorf("feed: unknown source %q", sourceID)
		}
	}
	return nil
}
