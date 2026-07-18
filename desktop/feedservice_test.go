package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFeedServiceItemsContract(t *testing.T) {
	t.Parallel()

	items := NewFeedService().Items("any-profile", "any-feed")

	require.Len(t, items, 6)
	assert.Equal(t, []string{"pr2841", "iss1190", "pr2838", "iss1204", "pr2830", "iss1177"}, itemIDs(items))
	assert.Equal(t, []string{"PR", "Issue", "PR", "Issue", "PR", "Issue"}, itemKinds(items))
	assert.Equal(t, []bool{true, true, false, true, false, false}, itemUnread(items))
	for _, item := range items {
		assert.NotEmpty(t, item.Branch, item.ID)
	}
}

func TestFeedServiceActionsForContract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind         string
		primaryTitle string
	}{
		{kind: "PR", primaryTitle: "Review PR"},
		{kind: "Issue", primaryTitle: "Start implementation"},
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			t.Parallel()

			actions := NewFeedService().ActionsFor(tt.kind)
			require.Len(t, actions, 4)

			primary := primaryActions(actions)
			require.Len(t, primary, 1)
			assert.Equal(t, tt.primaryTitle, primary[0].Title)
		})
	}
}

func TestFeedServiceProfilesContract(t *testing.T) {
	t.Parallel()

	profiles := NewFeedService().Profiles()

	require.NotEmpty(t, profiles)
	assert.Equal(t, 23, profiles[0].TotalCount)
	assert.Equal(t, 4, profiles[0].UnreadCount)
	assert.Len(t, profiles[0].Feeds, 3)
}

func itemIDs(items []FeedItem) []string {
	ids := make([]string, len(items))
	for i, item := range items {
		ids[i] = item.ID
	}
	return ids
}

func itemKinds(items []FeedItem) []string {
	kinds := make([]string, len(items))
	for i, item := range items {
		kinds[i] = item.Kind
	}
	return kinds
}

func itemUnread(items []FeedItem) []bool {
	unread := make([]bool, len(items))
	for i, item := range items {
		unread[i] = item.Unread
	}
	return unread
}

func primaryActions(actions []Action) []Action {
	primary := make([]Action, 0, 1)
	for _, action := range actions {
		if action.Primary {
			primary = append(primary, action)
		}
	}
	return primary
}
