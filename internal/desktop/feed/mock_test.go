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

func TestEmptyMockProviderCreatesWorkspace(t *testing.T) {
	t.Parallel()

	provider := NewEmptyMockProvider()

	profiles, err := provider.Profiles(t.Context())
	require.NoError(t, err)
	assert.Empty(t, profiles)

	sources, err := provider.Sources(t.Context())
	require.NoError(t, err)
	assert.Empty(t, sources, "onboarding starts without sources")

	_, err = provider.CreateProfile(t.Context(), "  ")
	require.Error(t, err)

	created, err := provider.CreateProfile(t.Context(), "My Triage")
	require.NoError(t, err)
	assert.Equal(t, "workspace-1", created.ID)
	assert.Equal(t, "My Triage", created.Name)
	assert.Equal(t, "M", created.Letter)
	require.NotEmpty(t, created.Feeds)

	profiles, err = provider.Profiles(t.Context())
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	assert.Equal(t, "workspace-1", profiles[0].ID)

	sources, err = provider.Sources(t.Context())
	require.NoError(t, err)
	assert.Len(t, sources, 3, "workspace creation seeds the default sources")
}

// TestMockProviderSourceAndFeedFlow walks the full editor flow the e2e
// mutable server must support: create a source, create a feed over it, read
// it back for edit prefill, and update it.
func TestMockProviderSourceAndFeedFlow(t *testing.T) {
	t.Parallel()

	provider := NewEmptyMockProvider()
	profile, err := provider.CreateProfile(t.Context(), "My Triage")
	require.NoError(t, err)

	src, err := provider.CreateSource(t.Context(), SourceDef{ID: "Team Reviews", Kind: "search", Query: "is:open review-requested:@me"})
	require.NoError(t, err)
	assert.Equal(t, "team-reviews", src.ID)

	dup, err := provider.CreateSource(t.Context(), SourceDef{ID: "team-reviews", Kind: "search", Query: "other"})
	require.NoError(t, err)
	assert.Equal(t, "team-reviews-2", dup.ID, "taken ids uniquify")

	_, err = provider.CreateSource(t.Context(), SourceDef{ID: "bad", Kind: "search"})
	require.ErrorContains(t, err, "requires a query")

	summary, err := provider.CreateFeed(t.Context(), profile.ID, FeedDef{
		Name:    "Team Reviews",
		Sources: []string{"team-reviews"},
		Filters: FilterDef{Types: []string{"pr"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "team-reviews", summary.ID)

	_, err = provider.CreateFeed(t.Context(), profile.ID, FeedDef{Name: "Broken", Sources: []string{"ghost"}})
	require.ErrorContains(t, err, "unknown source")
	_, err = provider.CreateFeed(t.Context(), "ghost", FeedDef{Name: "X", Sources: []string{"team-reviews"}})
	require.ErrorContains(t, err, "unknown profile")

	def, err := provider.FeedDefFor(t.Context(), profile.ID, "team-reviews")
	require.NoError(t, err)
	assert.Equal(t, "Team Reviews", def.Name)
	assert.Equal(t, []string{"pr"}, def.Filters.Types)

	err = provider.UpdateFeed(t.Context(), profile.ID, "team-reviews", FeedDef{
		Name:    "Reviews (renamed)",
		Sources: []string{"team-reviews", "team-reviews-2"},
	})
	require.NoError(t, err)

	def, err = provider.FeedDefFor(t.Context(), profile.ID, "team-reviews")
	require.NoError(t, err)
	assert.Equal(t, "team-reviews", def.ID, "feed keeps its id")
	assert.Equal(t, "Reviews (renamed)", def.Name)
	assert.Equal(t, []string{"team-reviews", "team-reviews-2"}, def.Sources)

	profiles, err := provider.Profiles(t.Context())
	require.NoError(t, err)
	var names []string
	for _, feedSummary := range profiles[0].Feeds {
		names = append(names, feedSummary.Name)
	}
	assert.Contains(t, names, "Reviews (renamed)", "profile summary reflects the rename")

	require.ErrorContains(t, provider.UpdateFeed(t.Context(), profile.ID, "ghost", FeedDef{Name: "X", Sources: []string{"team-reviews"}}), "unknown feed")
}

// TestMockProviderDeleteFlow walks the delete half of the editor flow the
// e2e mutable server must support: delete a feed, then delete the profile,
// leaving sources untouched throughout.
func TestMockProviderDeleteFlow(t *testing.T) {
	t.Parallel()

	provider := NewEmptyMockProvider()
	profile, err := provider.CreateProfile(t.Context(), "My Triage")
	require.NoError(t, err)
	require.NotEmpty(t, profile.Feeds)

	sourcesBefore, err := provider.Sources(t.Context())
	require.NoError(t, err)

	firstFeed := profile.Feeds[0].ID
	require.NoError(t, provider.DeleteFeed(t.Context(), profile.ID, firstFeed))

	profiles, err := provider.Profiles(t.Context())
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	for _, summary := range profiles[0].Feeds {
		assert.NotEqual(t, firstFeed, summary.ID, "deleted feed still in profile summary")
	}
	_, err = provider.FeedDefFor(t.Context(), profile.ID, firstFeed)
	require.ErrorContains(t, err, "unknown feed")

	require.ErrorContains(t, provider.DeleteFeed(t.Context(), profile.ID, "ghost"), "unknown feed")
	require.ErrorContains(t, provider.DeleteFeed(t.Context(), "ghost", firstFeed), "unknown profile")

	require.NoError(t, provider.DeleteProfile(t.Context(), profile.ID))
	profiles, err = provider.Profiles(t.Context())
	require.NoError(t, err)
	assert.Empty(t, profiles)

	// Sources are shared/decoupled: deleting the profile leaves them alone.
	sourcesAfter, err := provider.Sources(t.Context())
	require.NoError(t, err)
	assert.Equal(t, sourcesBefore, sourcesAfter)

	require.ErrorContains(t, provider.DeleteProfile(t.Context(), "ghost"), "unknown profile")
}

// TestMockProviderFeedDefsForFixture pins that the fixture profile's feed
// summaries all have definitions behind them, so the edit flow works on the
// canned data too.
func TestMockProviderFeedDefsForFixture(t *testing.T) {
	t.Parallel()

	provider := NewMockProvider()
	profiles, err := provider.Profiles(t.Context())
	require.NoError(t, err)
	require.NotEmpty(t, profiles)

	sources, err := provider.Sources(t.Context())
	require.NoError(t, err)
	require.NotEmpty(t, sources)
	known := make(map[string]bool, len(sources))
	for _, src := range sources {
		known[src.ID] = true
	}

	for _, summary := range profiles[0].Feeds {
		def, err := provider.FeedDefFor(t.Context(), profiles[0].ID, summary.ID)
		require.NoError(t, err, summary.ID)
		require.NotEmpty(t, def.Sources, summary.ID)
		for _, sourceID := range def.Sources {
			assert.True(t, known[sourceID], "feed %s references unknown source %s", summary.ID, sourceID)
		}
	}
}
