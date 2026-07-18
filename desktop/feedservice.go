package main

// FeedItem is an item from a profile feed.
type FeedItem struct {
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
}

// Profile is a desktop feed profile and its available sources.
type Profile struct {
	ID            string       `json:"id"`
	Letter        string       `json:"letter"`
	Name          string       `json:"name"`
	SourceSummary string       `json:"sourceSummary"`
	TotalCount    int          `json:"totalCount"`
	UnreadCount   int          `json:"unreadCount"`
	Feeds         []FeedSource `json:"feeds"`
}

// FeedSource is a selectable source within a profile.
type FeedSource struct {
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

// FeedService provides mock desktop feed data until GitHub-backed sources exist.
type FeedService struct{}

func NewFeedService() *FeedService {
	return &FeedService{}
}

// Profiles returns the profiles shown in the desktop rail. A single profile
// until profile creation exists.
func (s *FeedService) Profiles() []Profile {
	return []Profile{
		{
			ID:            "hive-core",
			Letter:        "H",
			Name:          "Frontend Triage",
			SourceSummary: "GitHub · 3 sources",
			TotalCount:    23,
			UnreadCount:   4,
			Feeds:         mockFeeds(),
		},
	}
}

func mockFeeds() []FeedSource {
	return []FeedSource{
		{ID: "my-open-prs", Name: "My open PRs", Count: 12, NewCount: 0},
		{ID: "notifications-inbox", Name: "Notifications inbox", Count: 3, NewCount: 3},
		{ID: "assigned-across-org", Name: "Assigned across org", Count: 8, NewCount: 0},
	}
}

// Items returns the mock feed items. Profile and feed IDs are reserved for the
// future GitHub-backed implementation.
func (s *FeedService) Items(profileID, feedID string) []FeedItem {
	return []FeedItem{
		{
			ID: "pr2841", Kind: "PR", Repo: "hive/core", Num: 2841,
			Title:  "batch_spawn: fix detached tmux env & PATH propagation",
			Author: "lena", Age: "2h", Unread: true, Labels: []string{"bug", "batch"},
			Branch: "fix/2841-batch-spawn-env",
			Body:   "Sessions spawned from a GUI context inherit an empty PATH and lose HIVE_* vars, so batch_spawn fails to find the agent binary. Needs a controlled env when there is no controlling terminal.",
			Prompt: "Investigate detached tmux env in batch_spawn; ensure PATH and HIVE_* propagate when spawned headless from the desktop app.",
		},
		{
			ID: "iss1190", Kind: "Issue", Repo: "hive/desktop", Num: 1190,
			Title:  "Feed source: mirror GitHub notifications inbox",
			Author: "hayden", Age: "5h", Unread: true, Labels: []string{"feature", "mvp"},
			Branch: "feat/1190-notifications-feed",
			Body:   "Add a notifications-based feed source that mirrors the user’s GitHub inbox, with local read/dismiss state so triage does not touch GitHub until the user acts.",
			Prompt: "Implement a notifications-based feed source mirroring the GitHub inbox, with app-local read/dismiss triage state.",
		},
		{
			ID: "pr2838", Kind: "PR", Repo: "hive/desktop", Num: 2838,
			Title:  "OAuth device flow for in-app GitHub auth",
			Author: "koji", Age: "1d", Unread: false, Labels: []string{"auth"},
			Branch: "feat/2838-oauth-device-flow",
			Body:   "Adds the full device-flow auth so users can sign in without leaving the app. Open question on GitHub App vs OAuth App registration and where to store the token.",
			Prompt: "Review the OAuth device-flow implementation and validate keychain token storage across platforms.",
		},
		{
			ID: "iss1204", Kind: "Issue", Repo: "hive/desktop", Num: 1204,
			Title:  "Composable view contract for feed / task / doc surfaces",
			Author: "mira", Age: "1d", Unread: true, Labels: []string{"arch"},
			Branch: "feat/1204-composable-views",
			Body:   "Define a self-contained component contract for feed, task, and doc views so a designer-led layout system can be dropped in later without rewrites.",
			Prompt: "Draft a composable, self-contained view interface covering the feed, task list, and doc viewer surfaces.",
		},
		{
			ID: "pr2830", Kind: "PR", Repo: "hive/core", Num: 2830,
			Title:  "Keychain-backed token storage",
			Author: "sam", Age: "2d", Unread: false, Labels: []string{"security"},
			Branch: "feat/2830-keychain-tokens",
			Body:   "Store GitHub tokens in the OS keychain instead of a plaintext config file, with a fallback for headless CI environments.",
			Prompt: "Review cross-platform keychain token storage and the headless fallback path.",
		},
		{
			ID: "iss1177", Kind: "Issue", Repo: "hive/desktop", Num: 1177,
			Title:  "Cross-repo query: PRs assigned to me across the org",
			Author: "hayden", Age: "3d", Unread: false, Labels: []string{"feature"},
			Branch: "feat/1177-cross-repo-query",
			Body:   "Support GitHub search-style cross-repo queries as a feed source, e.g. \"PRs assigned to me across the org\", saveable as a workspace source.",
			Prompt: "Implement a cross-repo query feed source using GitHub search syntax, saveable into a workspace.",
		},
	}
}

// ActionsFor returns the actions available for a PR or issue.
func (s *FeedService) ActionsFor(kind string) []Action {
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
