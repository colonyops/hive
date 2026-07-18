package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFeedServiceItemsContract(t *testing.T) {
	t.Parallel()

	service := NewFeedService()

	for _, profile := range service.Profiles() {
		items := service.Items(profile.ID, "")
		require.NotEmpty(t, items, "profile %s has no items", profile.ID)

		seenIDs := make(map[string]struct{}, len(items))
		for _, item := range items {
			assert.Contains(t, []string{"PR", "Issue"}, item.Kind, item.ID)
			assert.NotEmpty(t, item.ID)
			assert.NotEmpty(t, item.Title, item.ID)
			assert.NotEmpty(t, item.Branch, item.ID)

			_, duplicate := seenIDs[item.ID]
			assert.False(t, duplicate, "duplicate item ID %s", item.ID)
			seenIDs[item.ID] = struct{}{}
		}
	}
}

func TestFeedServiceActionsForContract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind string
	}{
		{kind: "PR"},
		{kind: "Issue"},
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			t.Parallel()

			actions := NewFeedService().ActionsFor(tt.kind)
			require.NotEmpty(t, actions)

			primary := primaryActions(actions)
			require.Len(t, primary, 1)

			for _, action := range actions {
				assert.NotEmpty(t, action.ID)
				assert.NotEmpty(t, action.Icon, action.ID)
				assert.NotEmpty(t, action.Color, action.ID)
				assert.NotEmpty(t, action.Title, action.ID)
				assert.NotEmpty(t, action.Sub, action.ID)
			}
		})
	}
}

func TestFeedServiceProfilesContract(t *testing.T) {
	t.Parallel()

	profiles := NewFeedService().Profiles()

	require.NotEmpty(t, profiles)
	for _, profile := range profiles {
		assert.NotEmpty(t, profile.ID)
		assert.NotEmpty(t, profile.Name, profile.ID)
		require.NotEmpty(t, profile.Feeds, profile.ID)
		for _, feed := range profile.Feeds {
			assert.NotEmpty(t, feed.ID, profile.ID)
			assert.NotEmpty(t, feed.Name, profile.ID)
		}
	}
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
