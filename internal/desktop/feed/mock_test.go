package feed

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The mock provider owns the fixture snapshot the e2e specs assert on; these
// tests pin structure, not literals.

func TestMockProviderItemsContract(t *testing.T) {
	t.Parallel()

	provider := NewMockProvider()
	profiles, err := provider.Profiles(t.Context())
	require.NoError(t, err)

	for _, profile := range profiles {
		items, err := provider.Items(t.Context(), profile.ID, "")
		require.NoError(t, err)
		require.NotEmpty(t, items, "profile %s has no items", profile.ID)

		seenIDs := make(map[string]struct{}, len(items))
		for _, item := range items {
			assert.Contains(t, []string{"PR", "Issue"}, item.Kind, item.ID)
			assert.NotEmpty(t, item.ID)
			assert.NotEmpty(t, item.Title, item.ID)
			assert.NotEmpty(t, item.Branch, item.ID)
			assert.NotEmpty(t, item.URL, item.ID)

			_, duplicate := seenIDs[item.ID]
			assert.False(t, duplicate, "duplicate item ID %s", item.ID)
			seenIDs[item.ID] = struct{}{}
		}
	}
}

func TestActionsForContract(t *testing.T) {
	t.Parallel()

	for _, kind := range []string{"PR", "Issue"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()

			actions := ActionsFor(kind)
			require.NotEmpty(t, actions)

			primary := 0
			for _, action := range actions {
				if action.Primary {
					primary++
				}
				assert.NotEmpty(t, action.ID)
				assert.NotEmpty(t, action.Icon, action.ID)
				assert.NotEmpty(t, action.Color, action.ID)
				assert.NotEmpty(t, action.Title, action.ID)
				assert.NotEmpty(t, action.Sub, action.ID)
			}
			assert.Equal(t, 1, primary, "exactly one primary action per kind")
		})
	}
}

func TestMockProviderProfilesContract(t *testing.T) {
	t.Parallel()

	profiles, err := NewMockProvider().Profiles(t.Context())
	require.NoError(t, err)

	require.NotEmpty(t, profiles)
	for _, profile := range profiles {
		assert.NotEmpty(t, profile.ID)
		assert.NotEmpty(t, profile.Name, profile.ID)
		require.NotEmpty(t, profile.Feeds, profile.ID)
		for _, source := range profile.Feeds {
			assert.NotEmpty(t, source.ID, profile.ID)
			assert.NotEmpty(t, source.Name, profile.ID)
		}
	}
}
