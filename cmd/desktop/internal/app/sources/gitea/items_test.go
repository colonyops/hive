package gitea

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/gitea/giteaclient"
)

func at(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return parsed
}

func TestSearchItemsCarryTheCanonicalContract(t *testing.T) {
	t.Parallel()

	items := searchItems([]giteaclient.Issue{{
		Number:     42,
		Title:      "Fix the checkout flow",
		Body:       "It drops the cart.",
		State:      "closed",
		HTMLURL:    "https://git.example.com/acme/app/pulls/42",
		UpdatedAt:  at("2026-08-11T22:10:05Z"),
		User:       giteaclient.User{Login: "octocat"},
		Labels:     []giteaclient.Label{{Name: "bug"}},
		Repository: giteaclient.RepoMeta{FullName: "acme/app"},
		PullReq:    &giteaclient.PullMeta{Merged: true},
	}})

	require.Len(t, items, 1)
	item := items[0]
	assert.Equal(t, "acme/app#42", item.ID)
	assert.Equal(t, "PR", item.Kind)
	assert.Equal(t, "acme/app", item.Repo)
	assert.Equal(t, 42, item.Num)
	assert.Equal(t, "octocat", item.Author)
	assert.Equal(t, "merged", item.State)
	assert.Equal(t, OriginSearch, item.Origin)
	assert.Equal(t, []string{"bug"}, item.Labels)
	assert.Equal(t, at("2026-08-11T22:10:05Z").UnixMilli(), item.UpdatedAt)
	assert.Equal(t, "https://git.example.com/acme/app/pulls/42", item.URL)
	assert.Equal(t, "gitea-pr-42-fix-the-checkout-flow", item.Branch)
	assert.Contains(t, item.Prompt, "Review pull request")
	assert.True(t, item.Unread, "a searched item is unread until it is read in the app")
}

func TestSearchItemsBranchAnIssueWithoutThePRPrefix(t *testing.T) {
	t.Parallel()

	items := searchItems([]giteaclient.Issue{{
		Number: 7, Title: "Add a setting", State: "open",
		Repository: giteaclient.RepoMeta{FullName: "acme/app"},
	}})
	require.Len(t, items, 1)
	assert.Equal(t, "Issue", items[0].Kind)
	assert.Equal(t, "gitea-7-add-a-setting", items[0].Branch)
	assert.Contains(t, items[0].Prompt, "Work on")
}

func TestNotificationItemsKeepTheSubjectState(t *testing.T) {
	t.Parallel()

	items := notificationItems([]giteaclient.NotificationThread{{
		Unread:     false,
		UpdatedAt:  at("2026-08-12T08:34:18Z"),
		Repository: giteaclient.Repository{FullName: "acme/app"},
		Subject: giteaclient.Subject{
			Title:   "Back up listmonk",
			Type:    "Pull",
			State:   "merged",
			URL:     "https://git.example.com/api/v1/repos/acme/app/issues/524",
			HTMLURL: "https://git.example.com/acme/app/pulls/524",
		},
	}})

	require.Len(t, items, 1)
	item := items[0]
	assert.Equal(t, "acme/app#524", item.ID)
	assert.Equal(t, "PR", item.Kind)
	assert.Equal(t, 524, item.Num)
	assert.Equal(t, "merged", item.State, "Gitea reports the subject's lifecycle on the notification itself")
	assert.Equal(t, OriginNotifications, item.Origin)
	assert.False(t, item.Unread)
	assert.Equal(t, "https://git.example.com/acme/app/pulls/524", item.URL)
}

// A commit or repository thread has no PR/Issue kind and no number, so it is
// dropped rather than ingested as an item with nothing to address.
func TestNotificationItemsDropSubjectsWithNoItem(t *testing.T) {
	t.Parallel()

	items := notificationItems([]giteaclient.NotificationThread{
		{Subject: giteaclient.Subject{Type: "Commit", URL: "https://git.example.com/api/v1/repos/acme/app/git/commits/abc"}},
		{Subject: giteaclient.Subject{Type: "Repository", URL: "https://git.example.com/api/v1/repos/acme/app"}},
		{Subject: giteaclient.Subject{Type: "Issue", URL: "https://git.example.com/api/v1/repos/acme/app/issues/nope"}},
	})
	assert.Empty(t, items)
}

func TestMergeItemsDeduplicatesAndOrdersByRecency(t *testing.T) {
	t.Parallel()

	older := Item{ID: "acme/app#1", UpdatedAt: 100}
	newer := Item{ID: "acme/app#2", UpdatedAt: 300}
	middle := Item{ID: "acme/app#3", UpdatedAt: 200}

	merged := mergeItems([][]Item{{older, newer}, {newer, middle}}, 10)

	require.Len(t, merged, 3, "an item matching two involvements is one item")
	assert.Equal(t, []string{"acme/app#2", "acme/app#3", "acme/app#1"}, ids(merged))
}

// The limit is what one node asked for, so a union of three searches still
// yields one page rather than three.
func TestMergeItemsTruncatesToTheLimit(t *testing.T) {
	t.Parallel()

	merged := mergeItems([][]Item{
		{{ID: "a", UpdatedAt: 100}, {ID: "b", UpdatedAt: 300}},
		{{ID: "c", UpdatedAt: 200}},
	}, 2)

	assert.Equal(t, []string{"b", "c"}, ids(merged))
}

func ids(items []Item) []string {
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = item.ID
	}
	return out
}
