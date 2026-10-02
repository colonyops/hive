package stores

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
)

func itemRef() models.ItemRef {
	return models.ItemRef{ProfileID: "p", SourceKind: "github", SourceScope: "acct", ExternalID: "acme/repo#1"}
}

func TestItemSessions_LinksAndListsNewestFirst(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()
	ref := itemRef()

	require.NoError(t, st.ItemSessions.Link(ctx, "sess-a", ref))
	require.NoError(t, st.ItemSessions.Link(ctx, "sess-b", ref))
	// Links are minted with the store's clock, so two created in the same test
	// would tie on created_at; pin them apart to assert the ordering itself.
	_, err := db.Conn().ExecContext(ctx, `UPDATE item_session SET created_at = 100 WHERE session_id = 'sess-a'`)
	require.NoError(t, err)
	_, err = db.Conn().ExecContext(ctx, `UPDATE item_session SET created_at = 200 WHERE session_id = 'sess-b'`)
	require.NoError(t, err)

	links, err := st.ItemSessions.List(ctx, ref)
	require.NoError(t, err)
	require.Len(t, links, 2)
	assert.Equal(t, "sess-b", links[0].SessionID)
	assert.Equal(t, "sess-a", links[1].SessionID)
}

func TestItemSessions_ScopedToTheItem(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()
	ref := itemRef()
	other := ref
	other.ExternalID = "acme/repo#2"

	require.NoError(t, st.ItemSessions.Link(ctx, "mine", ref))
	require.NoError(t, st.ItemSessions.Link(ctx, "theirs", other))

	links, err := st.ItemSessions.List(ctx, ref)
	require.NoError(t, err)
	require.Len(t, links, 1)
	assert.Equal(t, "mine", links[0].SessionID)
}

func TestLinkItemSession_IsIdempotent(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()
	ref := itemRef()

	require.NoError(t, st.ItemSessions.Link(ctx, "sess-a", ref))
	require.NoError(t, st.ItemSessions.Link(ctx, "sess-a", ref))

	links, err := st.ItemSessions.List(ctx, ref)
	require.NoError(t, err)
	assert.Len(t, links, 1)
}

// An action invoked from a surface with no inbox item behind it carries a zero
// ref, which must not become a link every such action shares.
func TestItemSessions_OneSessionLinksToMultipleItems(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()
	first := itemRef()
	second := first
	second.ExternalID = "acme/repo#2"

	require.NoError(t, st.ItemSessions.Link(ctx, "sess-a", first))
	require.NoError(t, st.ItemSessions.Link(ctx, "sess-a", second))

	firstLinks, err := st.ItemSessions.List(ctx, first)
	require.NoError(t, err)
	secondLinks, err := st.ItemSessions.List(ctx, second)
	require.NoError(t, err)
	require.Len(t, firstLinks, 1)
	require.Len(t, secondLinks, 1)
	assert.Equal(t, "sess-a", firstLinks[0].SessionID)
	assert.Equal(t, "sess-a", secondLinks[0].SessionID)
}

func TestLinkItemSession_IgnoresAnUnknownRef(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()

	require.NoError(t, st.ItemSessions.Link(ctx, "sess-a", models.ItemRef{}))
	require.NoError(t, st.ItemSessions.Link(ctx, "", itemRef()))

	var count int
	require.NoError(t, db.Conn().QueryRowContext(ctx, `SELECT count(*) FROM item_session`).Scan(&count))
	assert.Equal(t, 0, count)
}

func TestUnlinkItemSessions_DropsOnlyTheNamedSessions(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()
	ref := itemRef()

	require.NoError(t, st.ItemSessions.Link(ctx, "gone", ref))
	require.NoError(t, st.ItemSessions.Link(ctx, "kept", ref))
	require.NoError(t, st.ItemSessions.Unlink(ctx, []string{"gone"}))

	links, err := st.ItemSessions.List(ctx, ref)
	require.NoError(t, err)
	require.Len(t, links, 1)
	assert.Equal(t, "kept", links[0].SessionID)
}

// The row id is deliberately not what a link is keyed on: a replay deletes and
// rebuilds every row a profile owns, and an association must outlive that.
func TestItemSessions_SurviveAnItemRowBeingRebuilt(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()
	current := models.Observation{ExternalID: "acme/repo#1", Title: "one", SourceKind: "github", SourceScope: "acct", ObservedAt: 100, Payload: []byte(`{"v":1}`)}
	first, err := st.InboxItems.IngestObservation(ctx, activityClassifier("one"), IngestObservationParams{ProfileID: "p", Topic: "source:p/a", Current: current})
	require.NoError(t, err)

	ref, err := st.InboxItems.RefByID(ctx, first.ItemID)
	require.NoError(t, err)
	require.NoError(t, st.ItemSessions.Link(ctx, "sess-a", ref))

	_, err = db.Conn().ExecContext(ctx, `DELETE FROM inbox_item WHERE profile_id = 'p'`)
	require.NoError(t, err)
	// A rebuild re-observes the item; the payload has moved on, or the ingest
	// would match the source head and write nothing.
	current.Payload = []byte(`{"v":2}`)
	rebuilt, err := st.InboxItems.IngestObservation(ctx, activityClassifier("two"), IngestObservationParams{ProfileID: "p", Topic: "source:p/a", Current: current})
	require.NoError(t, err)
	require.NotEqual(t, first.ItemID, rebuilt.ItemID)

	rebuiltRef, err := st.InboxItems.RefByID(ctx, rebuilt.ItemID)
	require.NoError(t, err)
	links, err := st.ItemSessions.List(ctx, rebuiltRef)
	require.NoError(t, err)
	require.Len(t, links, 1)
	assert.Equal(t, "sess-a", links[0].SessionID)
}

// InboxItemStore.ResolveScoped rewrites the source_scope of a row from before
// hay-kot/hive-desktop#63 in
// place, and the links are keyed on that scope -- so they have to move with
// it.
func TestItemSessions_FollowALegacyRowOntoItsHealedScope(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()
	legacy, err := db.InsertInboxItem(ctx, queries.InsertInboxItemParams{
		ProfileID: "p", SourceKind: "github", SourceScope: "", ExternalID: "acme/repo#1",
		Payload: []byte(`{"v":1}`), Lifecycle: "active", Unread: 1,
	})
	require.NoError(t, err)

	legacyRef, err := st.InboxItems.RefByID(ctx, legacy.ID)
	require.NoError(t, err)
	require.NoError(t, st.ItemSessions.Link(ctx, "sess-a", legacyRef))

	current := models.Observation{ExternalID: "acme/repo#1", Title: "one", SourceKind: "github", SourceScope: "acct", ObservedAt: 100, Payload: []byte(`{"v":2}`)}
	_, err = st.InboxItems.IngestObservation(ctx, activityClassifier("one"), IngestObservationParams{ProfileID: "p", Topic: "source:p/a", Current: current})
	require.NoError(t, err)

	links, err := st.ItemSessions.List(ctx, itemRef())
	require.NoError(t, err)
	require.Len(t, links, 1)
	assert.Equal(t, "sess-a", links[0].SessionID)
}

func TestItemSessions_RescopeMergesAnExistingTargetLink(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()
	legacy := itemRef()
	legacy.SourceScope = ""
	scoped := itemRef()

	require.NoError(t, st.ItemSessions.Link(ctx, "sess-a", legacy))
	require.NoError(t, st.ItemSessions.Link(ctx, "sess-a", scoped))
	require.NoError(t, st.ItemSessions.Rescope(ctx, legacy.ProfileID, legacy.SourceKind, legacy.ExternalID, scoped.SourceScope))

	links, err := st.ItemSessions.List(ctx, scoped)
	require.NoError(t, err)
	require.Len(t, links, 1)
	assert.Equal(t, "sess-a", links[0].SessionID)

	var count int
	require.NoError(t, db.Conn().QueryRowContext(ctx, `SELECT count(*) FROM item_session`).Scan(&count))
	assert.Equal(t, 1, count)
}

func TestItemSessionStore_DeleteByProfile(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()
	ref := itemRef()
	other := ref
	other.ProfileID = "q"

	require.NoError(t, st.ItemSessions.Link(ctx, "gone", ref))
	require.NoError(t, st.ItemSessions.Link(ctx, "kept", other))

	require.NoError(t, st.ItemSessions.DeleteByProfile(ctx, "p"))

	links, err := st.ItemSessions.List(ctx, ref)
	require.NoError(t, err)
	assert.Empty(t, links)
	links, err = st.ItemSessions.List(ctx, other)
	require.NoError(t, err)
	assert.Len(t, links, 1)
}

func createChat(t *testing.T, st *Stores, name string) AgentSession {
	t.Helper()
	chat, err := st.AgentSessions.Create(t.Context(), AgentSessionCreate{Workspace: "triage", Name: name, Agent: "claude"})
	require.NoError(t, err)
	return chat
}

func TestItemChats_LinksListsAndFollowsTheChat(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()
	ref := itemRef()
	chat := createChat(t, st, "first")

	require.NoError(t, st.ItemSessions.LinkChat(ctx, chat.ID, ref))
	require.NoError(t, st.ItemSessions.LinkChat(ctx, chat.ID, ref))
	require.NoError(t, st.AgentSessions.Rename(ctx, chat.ID, "renamed"))

	chats, err := st.ItemSessions.ListChats(ctx, ref)
	require.NoError(t, err)
	require.Len(t, chats, 1)
	assert.Equal(t, ItemChat{ChatID: chat.ID, Workspace: "triage", Name: "renamed", CreatedAt: chats[0].CreatedAt}, chats[0])

	require.NoError(t, st.AgentSessions.Delete(ctx, chat.ID))
	chats, err = st.ItemSessions.ListChats(ctx, ref)
	require.NoError(t, err)
	assert.Empty(t, chats)
}

func TestItemChats_IgnoresAnUnknownItem(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()
	chat := createChat(t, st, "orphan")

	require.NoError(t, st.ItemSessions.LinkChat(ctx, chat.ID, models.ItemRef{}))
	chats, err := st.ItemSessions.ListChats(ctx, models.ItemRef{})
	require.NoError(t, err)
	assert.Empty(t, chats)
}

func TestItemChats_RescopeAndDeleteByProfile(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()
	scoped := itemRef()
	legacy := scoped
	legacy.SourceScope = ""
	chat := createChat(t, st, "legacy")

	require.NoError(t, st.ItemSessions.LinkChat(ctx, chat.ID, legacy))
	require.NoError(t, st.ItemSessions.Rescope(ctx, legacy.ProfileID, legacy.SourceKind, legacy.ExternalID, scoped.SourceScope))

	chats, err := st.ItemSessions.ListChats(ctx, scoped)
	require.NoError(t, err)
	require.Len(t, chats, 1)

	require.NoError(t, st.ItemSessions.DeleteByProfile(ctx, scoped.ProfileID))
	chats, err = st.ItemSessions.ListChats(ctx, scoped)
	require.NoError(t, err)
	assert.Empty(t, chats)
	_, err = st.AgentSessions.Get(ctx, chat.ID)
	require.NoError(t, err, "deleting links must leave the chat itself")
}
