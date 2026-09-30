package stores

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
)

func seedReplayItem(t *testing.T, db *queries.DB, profile, external string) queries.InboxItem {
	t.Helper()
	item, err := db.InsertInboxItem(t.Context(), queries.InsertInboxItemParams{
		ProfileID: profile, SourceKind: "github", SourceScope: "scope", ExternalID: external,
		Payload: []byte(`{}`), Lifecycle: "active",
	})
	require.NoError(t, err)
	return item
}

func activateReplay(t *testing.T, st *Stores, profile string, claims []models.FeedClaim) {
	t.Helper()
	feeds := make([]string, 0, len(claims))
	sources := make([]string, 0, len(claims))
	for _, claim := range claims {
		feeds = append(feeds, claim.FeedID)
		sources = append(sources, claim.SourceID)
	}
	tail, err := st.EventLog.TailOffset(t.Context())
	require.NoError(t, err)
	require.NoError(t, st.EventLog.ActivateReplay(t.Context(), profile, tail, claims, feeds, sources, nil))
}

func TestActivateReplay_OnlyWritesClaimsAndOffset(t *testing.T) {
	st, db := openTestStores(t)
	item := seedReplayItem(t, db, "flow", "item")
	activateReplay(t, st, "flow", []models.FeedClaim{{FeedID: "flow/feed", ItemID: item.ID, SourceID: "source:flow/a"}})
	var commands, offsets, claims int
	require.NoError(t, db.Conn().QueryRowContext(t.Context(), `SELECT COUNT(*) FROM output_command`).Scan(&commands))
	require.NoError(t, db.Conn().QueryRowContext(t.Context(), `SELECT COUNT(*) FROM consumer_offset`).Scan(&offsets))
	require.NoError(t, db.Conn().QueryRowContext(t.Context(), `SELECT COUNT(*) FROM feed_membership_claim`).Scan(&claims))
	assert.Zero(t, commands)
	assert.Equal(t, 1, offsets)
	assert.Equal(t, 1, claims)
}

func TestActivateReplay_RollsBackOffsetAndClaimsOnFailure(t *testing.T) {
	st, db := openTestStores(t)
	item := seedReplayItem(t, db, "flow", "item")
	require.NoError(t, st.FeedClaims.Upsert(t.Context(), models.FeedClaim{ProfileID: "flow", FeedID: "flow/old", ItemID: item.ID, SourceID: "source:flow/old"}))
	_, err := st.EventLog.Append(t.Context(), "source:flow/source", "item", []byte(`{}`))
	require.NoError(t, err)

	err = st.EventLog.ActivateReplay(t.Context(), "flow", 1, []models.FeedClaim{{ProfileID: "other", FeedID: "flow/new", ItemID: item.ID, SourceID: "source:flow/new"}}, []string{"flow/new"}, []string{"source:flow/new"}, nil)
	require.EqualError(t, err, `activating replay: claim profile "other" does not match "flow"`)

	var feed string
	require.NoError(t, db.Conn().QueryRowContext(t.Context(), `SELECT feed_id FROM feed_membership_claim WHERE item_id = ?`, item.ID).Scan(&feed))
	assert.Equal(t, "flow/old", feed)
	var offsets int
	require.NoError(t, db.Conn().QueryRowContext(t.Context(), `SELECT COUNT(*) FROM consumer_offset`).Scan(&offsets))
	assert.Zero(t, offsets)
}

func TestActivateReplay_ReplacesOnlyUnarchivedClaims(t *testing.T) {
	st, db := openTestStores(t)
	open := seedReplayItem(t, db, "flow", "open")
	archived := seedReplayItem(t, db, "flow", "archived")
	require.NoError(t, st.FeedClaims.Upsert(t.Context(), models.FeedClaim{ProfileID: "flow", FeedID: "flow/feed", ItemID: open.ID, SourceID: "source:flow/old"}))
	require.NoError(t, st.FeedClaims.Upsert(t.Context(), models.FeedClaim{ProfileID: "flow", FeedID: "flow/feed", ItemID: archived.ID, SourceID: "source:flow/old"}))
	_, err := db.Conn().ExecContext(t.Context(), `UPDATE inbox_item SET archived_at = 1, archived_actor = 'manual' WHERE id = ?`, archived.ID)
	require.NoError(t, err)

	require.NoError(t, st.EventLog.ActivateReplay(t.Context(), "flow", 0, []models.FeedClaim{{FeedID: "flow/new", ItemID: open.ID, SourceID: "source:flow/new"}}, []string{"flow/feed", "flow/new"}, []string{"source:flow/new"}, nil))
	rows, err := db.Conn().QueryContext(t.Context(), `SELECT profile_id, feed_id, item_id, source_id FROM feed_membership_claim ORDER BY item_id`)
	require.NoError(t, err)
	defer func() { require.NoError(t, rows.Close()) }()
	var got []string
	for rows.Next() {
		var profile, feed, source string
		var itemID int64
		require.NoError(t, rows.Scan(&profile, &feed, &itemID, &source))
		got = append(got, profile+":"+feed+"/"+source)
	}
	require.NoError(t, rows.Err())
	// The new claim carried no ProfileID; ActivateReplay must stamp it with
	// the activating profile rather than persist an empty one.
	assert.Equal(t, []string{"flow:flow/new/source:flow/new", "flow:flow/feed/source:flow/old"}, got)
}

func TestActivateReplay_DoesNotMutateInboxTriage(t *testing.T) {
	st, db := openTestStores(t)
	item := seedReplayItem(t, db, "flow", "item")
	_, err := db.Conn().ExecContext(t.Context(), `UPDATE inbox_item SET unread = 1, lifecycle = 'terminal', archived_reason = 'manual' WHERE id = ?`, item.ID)
	require.NoError(t, err)
	require.NoError(t, st.FeedClaims.Upsert(t.Context(), models.FeedClaim{ProfileID: "flow", FeedID: "flow/old", ItemID: item.ID, SourceID: "source:flow/old"}))

	// A narrowed replay removes the membership but must not treat the inbox row
	// as a fresh observation or alter its lifecycle/triage state.
	activateReplay(t, st, "flow", nil)
	var unread int64
	var lifecycle, reason string
	require.NoError(t, db.Conn().QueryRowContext(t.Context(), `SELECT unread, lifecycle, archived_reason FROM inbox_item WHERE id = ?`, item.ID).Scan(&unread, &lifecycle, &reason))
	assert.Equal(t, int64(1), unread)
	assert.Equal(t, "terminal", lifecycle)
	assert.Equal(t, "manual", reason)
	var claims int
	require.NoError(t, db.Conn().QueryRowContext(t.Context(), `SELECT COUNT(*) FROM feed_membership_claim WHERE item_id = ?`, item.ID).Scan(&claims))
	assert.Zero(t, claims)
}

func TestActivateReplay_NoFeedsDeletesAllClaims(t *testing.T) {
	st, db := openTestStores(t)
	open := seedReplayItem(t, db, "flow", "open")
	archived := seedReplayItem(t, db, "flow", "archived")
	_, err := db.Conn().ExecContext(t.Context(), `UPDATE inbox_item SET archived_at = 1, archived_actor = 'manual' WHERE id = ?`, archived.ID)
	require.NoError(t, err)
	for _, itemID := range []int64{open.ID, archived.ID} {
		require.NoError(t, st.FeedClaims.Upsert(t.Context(), models.FeedClaim{ProfileID: "flow", FeedID: "flow/removed", ItemID: itemID, SourceID: "source:flow/removed"}))
	}

	require.NoError(t, st.EventLog.ActivateReplay(t.Context(), "flow", 0, nil, nil, []string{"source:flow/live"}, nil))
	var claims int
	require.NoError(t, db.Conn().QueryRowContext(t.Context(), `SELECT COUNT(*) FROM feed_membership_claim WHERE profile_id = 'flow'`).Scan(&claims))
	assert.Zero(t, claims)
}

func TestActivateReplay_UsesRetentionSafeHighWaterMark(t *testing.T) {
	st, db := openTestStores(t)
	for _, key := range []string{"one", "two"} {
		_, err := st.EventLog.Append(t.Context(), "source:flow/a", key, []byte(`{}`))
		require.NoError(t, err)
	}
	tail, err := st.EventLog.TailOffset(t.Context())
	require.NoError(t, err)
	require.Equal(t, int64(2), tail)

	_, err = db.Conn().ExecContext(t.Context(), `DELETE FROM event_log`)
	require.NoError(t, err)
	var rows int
	require.NoError(t, db.Conn().QueryRowContext(t.Context(), `SELECT COUNT(*) FROM event_log`).Scan(&rows))
	require.Zero(t, rows)

	require.NoError(t, st.EventLog.ActivateReplay(t.Context(), "flow", tail, nil, nil, nil, nil))
	offset, err := st.EventLog.ConsumerOffset(t.Context(), "flow")
	require.NoError(t, err)
	assert.Equal(t, tail, offset)

	err = st.EventLog.ActivateReplay(t.Context(), "other-flow", tail+1, nil, nil, nil, nil)
	require.EqualError(t, err, `activating replay for "other-flow": supplied tail 3 exceeds current event log tail 2`)
}

func TestActivateReplay_ProtectsArchivedClaims(t *testing.T) {
	st, db := openTestStores(t)
	open := seedReplayItem(t, db, "flow", "open")
	archived := seedReplayItem(t, db, "flow", "archived")
	_, err := db.Conn().ExecContext(t.Context(), `UPDATE inbox_item SET archived_at = 1, archived_actor = 'manual' WHERE id = ?`, archived.ID)
	require.NoError(t, err)
	for _, id := range []int64{open.ID, archived.ID} {
		require.NoError(t, st.FeedClaims.Upsert(t.Context(), models.FeedClaim{ProfileID: "flow", FeedID: "flow/feed", ItemID: id, SourceID: "source:flow/removed"}))
	}
	require.NoError(t, st.EventLog.ActivateReplay(t.Context(), "flow", 0, nil, []string{"flow/feed"}, []string{"source:flow/live"}, nil))
	var ids []int64
	rows, err := db.Conn().QueryContext(t.Context(), `SELECT item_id FROM feed_membership_claim ORDER BY item_id`)
	require.NoError(t, err)
	defer func() { require.NoError(t, rows.Close()) }()
	for rows.Next() {
		var id int64
		require.NoError(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, []int64{archived.ID}, ids)
}

func TestActivateReplay_ReconcilesNodeKV(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()

	require.NoError(t, st.NodeKV.Set(ctx, "flow", "kept", "seen", `1`, 0))
	require.NoError(t, st.NodeKV.Set(ctx, "flow", "removed", "seen", `1`, 0))
	require.NoError(t, st.NodeKV.Set(ctx, "other", "kept", "seen", `1`, 0))

	// "kept" stays — this also models a rename and a same-id recreate, both of
	// which preserve the id and therefore the KV.
	require.NoError(t, st.EventLog.ActivateReplay(ctx, "flow", 0, nil, nil, nil, []string{"kept"}))

	_, found, err := st.NodeKV.Get(ctx, "flow", "kept", "seen", 1)
	require.NoError(t, err)
	assert.True(t, found)
	_, found, err = st.NodeKV.Get(ctx, "flow", "removed", "seen", 1)
	require.NoError(t, err)
	assert.False(t, found)
	_, found, err = st.NodeKV.Get(ctx, "other", "kept", "seen", 1)
	require.NoError(t, err)
	assert.True(t, found, "another flow's rows are untouched")

	require.NoError(t, st.EventLog.ActivateReplay(ctx, "flow", 0, nil, nil, nil, nil))
	_, found, err = st.NodeKV.Get(ctx, "flow", "kept", "seen", 1)
	require.NoError(t, err)
	assert.False(t, found)
}

func TestActivateReplay_FailureLeavesNodeKVIntact(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()
	item := seedReplayItem(t, db, "flow", "item")

	require.NoError(t, st.NodeKV.Set(ctx, "flow", "old", "seen", `1`, 0))

	// The mismatched claim profile fails the transaction after the KV
	// reconcile would have cleared "old".
	err := st.EventLog.ActivateReplay(ctx, "flow", 0,
		[]models.FeedClaim{{ProfileID: "other", FeedID: "flow/f", ItemID: item.ID, SourceID: "source:flow/s"}},
		nil, nil, []string{"survivor"})
	require.Error(t, err)

	_, found, err := st.NodeKV.Get(ctx, "flow", "old", "seen", 1)
	require.NoError(t, err)
	assert.True(t, found, "a failed activation must leave last-known-good KV")
}
