package feed

import (
	"context"
	"fmt"
)

// MockProvider serves the fixture data the e2e suite snapshots. It is the
// provider in HIVE_DESKTOP_MOCK modes; MarkRead is a no-op so repeated e2e
// interactions never mutate the fixtures.
type MockProvider struct{}

func NewMockProvider() *MockProvider {
	return &MockProvider{}
}

func (p *MockProvider) Profiles(context.Context) ([]Profile, error) {
	return []Profile{
		{
			ID:            "hive-core",
			Letter:        "H",
			Name:          "Frontend Triage",
			SourceSummary: "GitHub · 3 sources",
			TotalCount:    23,
			UnreadCount:   4,
			Feeds: []Source{
				{ID: "my-open-prs", Name: "My open PRs", Count: 12, NewCount: 0},
				{ID: "notifications-inbox", Name: "Notifications inbox", Count: 3, NewCount: 3},
				{ID: "assigned-across-org", Name: "Assigned across org", Count: 8, NewCount: 0},
			},
		},
	}, nil
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
			Author: "hayden", Age: "5h", Unread: true, Labels: []string{"feature", "mvp"},
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

// CreateProfile is unsupported in mock mode: the fixture profile always
// exists, so onboarding step 2 is unreachable there.
func (p *MockProvider) CreateProfile(context.Context, string) (Profile, error) {
	return Profile{}, fmt.Errorf("feed: mock provider cannot create profiles")
}
