// Package feed is the desktop feed data layer: the wire types the frontend
// consumes, the Provider seam the Wails feed service delegates to, and its
// implementations (mock fixtures for tests/e2e, GitHub-backed live data).
package feed

import "context"

// Item is an item from a profile feed.
type Item struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"` // "PR" | "Issue"
	Repo   string `json:"repo"`
	Num    int    `json:"num"`
	Title  string `json:"title"`
	Author string `json:"author"`
	Age    string `json:"age"`
	Unread bool   `json:"unread"`
	// Reason is the GitHub notification reason (e.g. "review_requested"),
	// empty for items known only from search.
	Reason string   `json:"reason,omitempty"`
	Labels []string `json:"labels"`
	Branch string   `json:"branch"`
	Body   string   `json:"body"`
	Prompt string   `json:"prompt"`
	URL    string   `json:"url"`
}

// Profile is a desktop feed profile (a "workspace" in the designs) and its
// feeds.
type Profile struct {
	ID            string   `json:"id"`
	Letter        string   `json:"letter"`
	Name          string   `json:"name"`
	SourceSummary string   `json:"sourceSummary"`
	TotalCount    int      `json:"totalCount"`
	UnreadCount   int      `json:"unreadCount"`
	Feeds         []Source `json:"feeds"`
}

// Source is a selectable feed within a profile.
type Source struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Count    int    `json:"count"`
	NewCount int    `json:"newCount"`
}

// Action is an action offered for a feed item's kind.
type Action struct {
	ID      string `json:"id"`
	Icon    string `json:"icon"`
	Color   string `json:"color"`
	Title   string `json:"title"`
	Sub     string `json:"sub"`
	Primary bool   `json:"primary"`
}

// Provider supplies feed data to the desktop feed service.
type Provider interface {
	Profiles(ctx context.Context) ([]Profile, error)
	// Items returns the items of one feed, or of every feed in the profile
	// when feedID is "".
	Items(ctx context.Context, profileID, feedID string) ([]Item, error)
	ActionsFor(kind string) []Action
	// MarkRead records the item as read app-locally. Implementations must
	// not touch GitHub: triage state stays local until the user acts.
	MarkRead(ctx context.Context, profileID, itemID string) error
	// CreateProfile persists a new profile ("workspace") seeded with the
	// default feeds — onboarding step 2.
	CreateProfile(ctx context.Context, name string) (Profile, error)
	// Refresh refetches the distinct sources the profile's feeds reference,
	// bypassing the search cache TTL, so a manual "Refresh now" is a real
	// fetch. It reports whether anything changed and fails only when every
	// source fails.
	Refresh(ctx context.Context, profileID string) (bool, error)
	// Config describes the profiles config file backing the provider —
	// path, content, and validity — for the feeds-as-code UI.
	Config(ctx context.Context) (ConfigInfo, error)
	// Sources returns the top-level source definitions, for the feed
	// editor's source picker.
	Sources(ctx context.Context) ([]SourceDef, error)
	// FeedDefFor returns one feed's definition, for edit prefill.
	FeedDefFor(ctx context.Context, profileID, feedID string) (FeedDef, error)
	// CreateSource persists a new top-level source and returns it with its
	// assigned ID.
	CreateSource(ctx context.Context, def SourceDef) (SourceDef, error)
	// CreateFeed persists a new feed in the profile — its ID is derived
	// from the name — and returns the feed's materialized summary.
	CreateFeed(ctx context.Context, profileID string, def FeedDef) (Source, error)
	// UpdateFeed replaces the feed's definition; the feed keeps its ID.
	UpdateFeed(ctx context.Context, profileID, feedID string, def FeedDef) error
}

// ActionsFor returns the actions available for a PR or issue. Shared by
// every provider: actions are kind-scoped, not data-scoped, until the
// config-driven actions editor exists.
func ActionsFor(kind string) []Action {
	if kind == "PR" {
		return []Action{
			{ID: "review-pr", Icon: "play", Color: "#34d399", Title: "Review PR", Sub: "sonnet · read the diff, leave inline comments", Primary: true},
			{ID: "rethink-approach", Icon: "rotate-ccw", Color: "#a78bfa", Title: "Rethink approach", Sub: "opus · re-evaluate design, propose an alternative"},
			{ID: "summarize-changes", Icon: "list", Color: "#60a5fa", Title: "Summarize changes", Sub: "haiku · post a plain-language summary comment"},
			{ID: "reproduce-and-fix", Icon: "sparkles", Color: "#fb7185", Title: "Reproduce & fix", Sub: "sonnet · check out branch, reproduce, patch"},
		}
	}

	return []Action{
		{ID: "start-implementation", Icon: "play", Color: "#34d399", Title: "Start implementation", Sub: "sonnet · new branch, implement from the issue", Primary: true},
		{ID: "investigate", Icon: "search", Color: "#60a5fa", Title: "Investigate", Sub: "sonnet · research and write a findings doc"},
		{ID: "rethink-spec", Icon: "rotate-ccw", Color: "#a78bfa", Title: "Rethink / spec", Sub: "opus · draft a plan before writing code"},
		{ID: "create-hc-task", Icon: "diamond", Color: "#f59e0b", Title: "Create hc task", Sub: "turn this into a Honeycomb epic"},
	}
}
