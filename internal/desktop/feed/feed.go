// Package feed is the desktop feed data layer: the wire types the frontend
// consumes, the Provider seam the Wails feed service delegates to, and its
// implementations (mock fixtures for tests/e2e, GitHub-backed live data).
package feed

import "context"

// Item is an item from a profile feed.
type Item struct {
	ID     string   `json:"id"`
	Kind   string   `json:"kind"` // "PR" | "Issue"
	Repo   string   `json:"repo"`
	Num    int      `json:"num"`
	Title  string   `json:"title"`
	Author string   `json:"author"`
	Age    string   `json:"age"`
	Unread bool     `json:"unread"`
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
