package stores

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
)

func seedLookupItem(t *testing.T, database *queries.DB, ctx context.Context, externalID string) int64 {
	t.Helper()
	item, err := database.InsertInboxItem(ctx, queries.InsertInboxItemParams{
		ProfileID: "flow-1", SourceKind: "github", SourceScope: "source-a", ExternalID: externalID,
		Payload: []byte(`{"v":1}`), Lifecycle: "active",
	})
	require.NoError(t, err)
	return item.ID
}

func TestInboxItemStore_IDByExternalID(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()
	want := seedLookupItem(t, db, ctx, "item-1")

	got, err := st.InboxItems.IDByExternalID(ctx, "flow-1", "github", "source-a", "item-1")
	require.NoError(t, err)
	assert.Equal(t, want, got)

	// An identity with no inbox row is "nothing to link to", not a failure:
	// a notification about it still fires, it just has nothing to reveal.
	for _, missing := range [][4]string{
		{"flow-1", "github", "source-a", "item-missing"},
		{"flow-1", "github", "other-source", "item-1"},
		{"other-flow", "github", "source-a", "item-1"},
		{"flow-1", "github", "source-a", ""},
	} {
		got, err := st.InboxItems.IDByExternalID(ctx, missing[0], missing[1], missing[2], missing[3])
		require.NoError(t, err, missing)
		assert.Zero(t, got, missing)
	}
}

// The commit path resolves ids only, and an unscoped row from before hay-kot/hive-desktop#63 has to heal
// onto its scope there exactly as it does through ResolveScoped.
func TestInboxItemStore_ResolveScopedID(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()
	want := seedLookupItem(t, db, ctx, "item-1")

	got, err := st.InboxItems.ResolveScopedID(ctx, "flow-1", "github", "source-a", "item-1")
	require.NoError(t, err)
	assert.Equal(t, want, got)

	_, err = st.InboxItems.ResolveScopedID(ctx, "flow-1", "github", "source-a", "item-missing")
	require.ErrorIs(t, err, sql.ErrNoRows)

	legacy, err := db.InsertInboxItem(ctx, queries.InsertInboxItemParams{
		ProfileID: "flow-1", SourceKind: "github", SourceScope: "", ExternalID: "item-legacy",
		Payload: []byte(`{"v":1}`), Lifecycle: "active",
	})
	require.NoError(t, err)

	got, err = st.InboxItems.ResolveScopedID(ctx, "flow-1", "github", "source-a", "item-legacy")
	require.NoError(t, err)
	assert.Equal(t, legacy.ID, got)
	ref, err := st.InboxItems.RefByID(ctx, legacy.ID)
	require.NoError(t, err)
	assert.Equal(t, "source-a", ref.SourceScope)
}

func TestInboxItemStore_FindByExternalID(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()
	insert := func(profile, scope, ext string) {
		_, err := db.InsertInboxItem(ctx, queries.InsertInboxItemParams{
			ProfileID: profile, SourceKind: "github", SourceScope: scope, ExternalID: ext,
			Payload: []byte(`{"v":1}`), Lifecycle: "active",
		})
		require.NoError(t, err)
	}
	insert("p1", "s-a", "PR_1")
	insert("p1", "s-b", "PR_1")
	insert("p2", "s-a", "PR_1")
	insert("p1", "s-a", "PR_2")

	all, err := st.InboxItems.FindByExternalID(ctx, "", "PR_1")
	require.NoError(t, err)
	assert.Len(t, all, 3, "one external id spans profiles and source scopes")

	scoped, err := st.InboxItems.FindByExternalID(ctx, "p1", "PR_1")
	require.NoError(t, err)
	assert.Len(t, scoped, 2)
	for _, item := range scoped {
		assert.Equal(t, "p1", item.ProfileID)
	}

	none, err := st.InboxItems.FindByExternalID(ctx, "", "missing")
	require.NoError(t, err)
	assert.Empty(t, none)
}

func TestInboxItemStore_ListAll(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()
	insert := func(profile, ext string) {
		_, err := db.InsertInboxItem(ctx, queries.InsertInboxItemParams{
			ProfileID: profile, SourceKind: "github", SourceScope: "s", ExternalID: ext,
			Payload: []byte(`{"v":1}`), Lifecycle: "active",
		})
		require.NoError(t, err)
	}
	insert("p1", "a")
	insert("p1", "b")
	insert("p2", "c")

	all, err := st.InboxItems.ListAll(ctx, "", 10)
	require.NoError(t, err)
	assert.Len(t, all, 3)

	scoped, err := st.InboxItems.ListAll(ctx, "p1", 10)
	require.NoError(t, err)
	assert.Len(t, scoped, 2)

	empty, err := st.InboxItems.ListAll(ctx, "", 0)
	require.NoError(t, err)
	assert.Empty(t, empty, "a non-positive limit short-circuits")
}

func TestInboxItemStore_FeedIDForItem(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()
	itemID := seedLookupItem(t, db, ctx, "item-1")

	feedID, err := st.InboxItems.FeedIDForItem(ctx, "flow-1", itemID)
	require.NoError(t, err)
	assert.Empty(t, feedID)

	for _, claim := range []string{"flow-1/team", "flow-1/all"} {
		require.NoError(t, st.FeedClaims.Upsert(ctx, models.FeedClaim{
			ProfileID: "flow-1", FeedID: claim, ItemID: itemID, SourceID: "source:flow-1/source-a",
		}))
	}
	feedID, err = st.InboxItems.FeedIDForItem(ctx, "flow-1", itemID)
	require.NoError(t, err)
	assert.Equal(t, "flow-1/all", feedID)

	feedID, err = st.InboxItems.FeedIDForItem(ctx, "other-flow", itemID)
	require.NoError(t, err)
	assert.Empty(t, feedID)
}

// TestInboxItemStore_ListUnarchivedIsWailsSafe guards the wire shape
// InboxItem must keep: camelCase JSON tags, and a payload that marshals as
// the JSON it holds rather than a base64 byte string.
func TestInboxItemStore_ListUnarchivedIsWailsSafe(t *testing.T) {
	st, db := openTestStores(t)
	item, err := db.InsertInboxItem(t.Context(), queries.InsertInboxItemParams{
		ProfileID: "flow", SourceKind: "github", SourceScope: "scope", ExternalID: "item",
		Payload: []byte(`{}`), Lifecycle: "active",
	})
	require.NoError(t, err)

	items, err := st.InboxItems.ListUnarchived(t.Context(), "flow")
	require.NoError(t, err)
	require.Equal(t, []InboxItem{{ID: item.ID, ProfileID: "flow", SourceKind: "github", SourceScope: "scope", ExternalID: "item", Payload: []byte(`{}`), Lifecycle: "active", Revision: 1}}, items)
	encoded, err := json.Marshal(items[0])
	require.NoError(t, err)
	var wire map[string]any
	require.NoError(t, json.Unmarshal(encoded, &wire))
	assert.Equal(t, map[string]any{}, wire["payload"])
	assert.Equal(t, "flow", wire["profileId"])
	assert.NotContains(t, wire, "profile_id")
}

func TestInboxItemStore_FeedIDsForItems(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()
	a := seedLookupItem(t, db, ctx, "item-a")
	b := seedLookupItem(t, db, ctx, "item-b")
	c := seedLookupItem(t, db, ctx, "item-c")

	require.NoError(t, st.FeedClaims.Upsert(ctx, models.FeedClaim{ProfileID: "flow-1", FeedID: "flow-1/team", ItemID: a, SourceID: "source:flow-1/source-a"}))
	require.NoError(t, st.FeedClaims.Upsert(ctx, models.FeedClaim{ProfileID: "flow-1", FeedID: "flow-1/all", ItemID: b, SourceID: "source:flow-1/source-a"}))

	feeds, err := st.InboxItems.FeedIDsForItems(ctx, []int64{a, b, c})
	require.NoError(t, err)
	assert.Equal(t, "flow-1/team", feeds[a])
	assert.Equal(t, "flow-1/all", feeds[b])
	_, ok := feeds[c]
	assert.False(t, ok, "an item with no claim is absent from the map")

	empty, err := st.InboxItems.FeedIDsForItems(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, empty)
}
