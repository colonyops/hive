package queries

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
)

func TestPrune_EventLogIsNotGatedByConsumerOffsets(t *testing.T) {
	database := openTestDB(t)
	ctx := t.Context()

	for i := range 5 {
		_, err := database.Append(ctx, "source:test", fmt.Sprintf("key-%d", i), []byte(`{}`))
		require.NoError(t, err)
	}
	require.NoError(t, database.CommitConsumerOffset(ctx, CommitConsumerOffsetParams{Consumer: "fast-flow", Offset: 5}))
	require.NoError(t, database.CommitConsumerOffset(ctx, CommitConsumerOffsetParams{Consumer: "slow-flow", Offset: 2}))

	require.NoError(t, database.Prune(ctx, DefaultRetentionPolicy()))

	msgs, _, err := database.ReadFrom(ctx, 0, 10)
	require.NoError(t, err)
	require.Len(t, msgs, 5)
	assert.Equal(t, []string{"1", "2", "3", "4", "5"}, []string{msgs[0].ID, msgs[1].ID, msgs[2].ID, msgs[3].ID, msgs[4].ID})
}

func TestPrune_EventLogUsesAgeWithoutConsumers(t *testing.T) {
	database := openTestDB(t)
	ctx := t.Context()
	_, err := database.Append(ctx, "source:test", "old", []byte(`{}`))
	require.NoError(t, err)
	_, err = database.Append(ctx, "source:test", "new", []byte(`{}`))
	require.NoError(t, err)
	_, err = database.Conn().ExecContext(ctx, `UPDATE event_log SET created_at = ? WHERE "offset" = 1`, time.Now().Add(-2*time.Hour).UnixMilli())
	require.NoError(t, err)
	_, err = database.Conn().ExecContext(ctx, `UPDATE event_log SET created_at = ? WHERE "offset" = 2`, time.Now().UnixMilli())
	require.NoError(t, err)

	err = database.Prune(ctx, RetentionPolicy{EventLogMaxAge: time.Hour})
	require.NoError(t, err)
	msgs, _, err := database.ReadFrom(ctx, 0, 10)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "2", msgs[0].ID)
}

func TestPrune_EventLogUsesPerTopicCountWithoutConsumers(t *testing.T) {
	database := openTestDB(t)
	ctx := t.Context()

	for i := range 3 {
		_, err := database.Append(ctx, "source:test", fmt.Sprintf("key-%d", i), []byte(`{}`))
		require.NoError(t, err)
	}
	require.NoError(t, database.CommitConsumerOffset(ctx, CommitConsumerOffsetParams{Consumer: "committed", Offset: 3}))

	err := database.Prune(ctx, RetentionPolicy{EventLogPerTopicLimit: 1})
	require.NoError(t, err)

	msgs, _, err := database.ReadFrom(ctx, 0, 10)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "3", msgs[0].ID)
}

func TestPrune_PreservesLatestSourceSnapshotForReplay(t *testing.T) {
	database := openTestDB(t)
	ctx := t.Context()
	_, err := database.AppendSnapshot(ctx, "source:flow/source", "github", "", []models.SnapshotItem{{Key: "old", Payload: []byte(`{}`)}})
	require.NoError(t, err)
	latest, err := database.AppendSnapshot(ctx, "source:flow/source", "github", "", []models.SnapshotItem{{Key: "current", Payload: []byte(`{}`)}})
	require.NoError(t, err)
	_, err = database.Conn().ExecContext(ctx, `UPDATE event_log SET created_at = ?`, time.Now().Add(-2*time.Hour).UnixMilli())
	require.NoError(t, err)

	err = database.Prune(ctx, RetentionPolicy{EventLogMaxAge: time.Hour})
	require.NoError(t, err)
	messages, err := database.ListReplaySourceSnapshots(ctx, "flow", latest)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	assert.Equal(t, fmt.Sprint(latest), messages[0].ID)
	assert.Equal(t, "current", messages[0].Snapshot[0].Key)

	_, err = database.Append(ctx, "source:flow/source", "new-1", []byte(`{}`))
	require.NoError(t, err)
	newest, err := database.Append(ctx, "source:flow/source", "new-2", []byte(`{}`))
	require.NoError(t, err)
	err = database.Prune(ctx, RetentionPolicy{EventLogPerTopicLimit: 1})
	require.NoError(t, err)
	rows, _, err := database.ReadFrom(ctx, 0, 10)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, []string{fmt.Sprint(latest), fmt.Sprint(newest)}, []string{rows[0].ID, rows[1].ID})
}

func TestPrune_KeepsNewestSnapshotsPerTopicAndSparesItemEvents(t *testing.T) {
	database := openTestDB(t)
	ctx := t.Context()

	var snapshots []int64
	for i := range 5 {
		off, err := database.AppendSnapshot(ctx, "source:flow/a", "github", "acct", []models.SnapshotItem{{Key: fmt.Sprintf("s%d", i), Payload: []byte(`{}`)}})
		require.NoError(t, err)
		snapshots = append(snapshots, off)
	}
	item, err := database.Append(ctx, "source:flow/a", "item", []byte(`{}`))
	require.NoError(t, err)
	// A second topic's lone snapshot must be untouched: the bound is per topic.
	otherSnap, err := database.AppendSnapshot(ctx, "source:flow/b", "github", "acct", []models.SnapshotItem{{Key: "b", Payload: []byte(`{}`)}})
	require.NoError(t, err)

	err = database.Prune(ctx, RetentionPolicy{EventLogSnapshotsPerTopicLimit: 2})
	require.NoError(t, err)

	var kept []int64
	rows, err := database.Conn().QueryContext(ctx, `SELECT "offset" FROM event_log WHERE snapshot = 1 AND topic = 'source:flow/a' ORDER BY "offset"`)
	require.NoError(t, err)
	defer func() { require.NoError(t, rows.Close()) }()
	for rows.Next() {
		var off int64
		require.NoError(t, rows.Scan(&off))
		kept = append(kept, off)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, []int64{snapshots[3], snapshots[4]}, kept, "only the two newest snapshots for the topic survive")

	var itemEvents, otherSnaps int
	require.NoError(t, database.Conn().QueryRowContext(ctx, `SELECT COUNT(*) FROM event_log WHERE "offset" = ? AND snapshot = 0`, item).Scan(&itemEvents))
	require.NoError(t, database.Conn().QueryRowContext(ctx, `SELECT COUNT(*) FROM event_log WHERE "offset" = ?`, otherSnap).Scan(&otherSnaps))
	assert.Equal(t, 1, itemEvents, "item events are not touched by the snapshot bound")
	assert.Equal(t, 1, otherSnaps, "a different topic's snapshots are bounded independently")
}

func TestPrune_RejectsNegativeSnapshotsPerTopicLimit(t *testing.T) {
	database := openTestDB(t)
	err := database.Prune(t.Context(), RetentionPolicy{EventLogSnapshotsPerTopicLimit: -1})
	require.EqualError(t, err, "event log snapshots per-topic limit must not be negative")
}

func TestPrune_BoundsOnlyTerminalHistory(t *testing.T) {
	database := openTestDB(t)
	ctx := t.Context()

	for i := range 3 {
		_, err := database.Conn().ExecContext(ctx, `
			INSERT INTO node_run (flow_id, node_id, ok, in_count, out_count, drop_count, ended_at, dur_ms)
			VALUES ('flow', ?, 1, 0, 0, 0, ?, 0)
		`, fmt.Sprintf("node-%d", i), i+1)
		require.NoError(t, err)
	}
	for i, status := range []string{"done", "failed", "done", "pending", "running"} {
		_, err := database.Conn().ExecContext(ctx, `
			INSERT INTO output_command (action_id, key, payload, status, created_at)
			VALUES (?, ?, X'7B7D', ?, ?)
		`, fmt.Sprintf("action-%d", i), fmt.Sprintf("key-%d", i), status, i)
		require.NoError(t, err)
	}
	for i, status := range []string{"done", "failed", "done", "queued", "running"} {
		_, err := database.InsertJob(ctx, InsertJobParams{
			CreatedAt: int64(i), UpdatedAt: int64(i), Status: status, Label: fmt.Sprintf("job-%d", i),
		})
		require.NoError(t, err)
	}

	err := database.Prune(ctx, RetentionPolicy{
		NodeRunLimit: 2, TerminalOutputCommandLimit: 2, JobLimit: 2,
	})
	require.NoError(t, err)

	var nodeRuns int
	require.NoError(t, database.Conn().QueryRowContext(ctx, `SELECT COUNT(*) FROM node_run`).Scan(&nodeRuns))
	assert.Equal(t, 2, nodeRuns)

	var statuses []string
	rows, err := database.Conn().QueryContext(ctx, `SELECT status FROM output_command ORDER BY id`)
	require.NoError(t, err)
	defer func() { require.NoError(t, rows.Close()) }()
	for rows.Next() {
		var status string
		require.NoError(t, rows.Scan(&status))
		statuses = append(statuses, status)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, []string{"failed", "done", "pending", "running"}, statuses)

	jobs, err := database.ListJobs(ctx, ListJobsParams{ID: math.MaxInt64, Limit: 10})
	require.NoError(t, err)
	require.Len(t, jobs, 4)
	assert.Equal(t, []string{"running", "queued", "done", "failed"}, []string{
		jobs[0].Status, jobs[1].Status, jobs[2].Status, jobs[3].Status,
	})
}

func TestPrune_RejectsNegativeJobLimit(t *testing.T) {
	database := openTestDB(t)
	err := database.Prune(t.Context(), RetentionPolicy{JobLimit: -1})
	require.EqualError(t, err, "job retention limit must not be negative")
}

func TestPrune_PrunesExpiredArchivedInboxItemsAndCascades(t *testing.T) {
	database := openTestDB(t)
	ctx := t.Context()
	item := seedInboxItem(t, database, "flow", "expired")
	_, err := database.InsertInboxEvent(ctx, InsertInboxEventParams{
		ItemID: item.ID, Kind: "updated", Transition: "none", Attention: "activity", Detail: []byte(`{}`), CreatedAt: 1,
	})
	require.NoError(t, err)
	require.NoError(t, database.UpsertFeedMembershipClaim(ctx, UpsertFeedMembershipClaimParams{
		ProfileID: "flow", FeedID: "flow/feed", ItemID: item.ID, SourceID: "source:flow/source",
	}))
	_, err = database.Conn().ExecContext(ctx, `UPDATE inbox_item SET archived_at = ?, archived_actor = 'manual' WHERE id = ?`, time.Now().Add(-91*24*time.Hour).UnixMilli(), item.ID)
	require.NoError(t, err)

	err = database.Prune(ctx, RetentionPolicy{ArchivedItemRetention: 90 * 24 * time.Hour})
	require.NoError(t, err)

	var items, events, claims int
	require.NoError(t, database.Conn().QueryRowContext(ctx, `SELECT COUNT(*) FROM inbox_item WHERE id = ?`, item.ID).Scan(&items))
	require.NoError(t, database.Conn().QueryRowContext(ctx, `SELECT COUNT(*) FROM inbox_event WHERE item_id = ?`, item.ID).Scan(&events))
	require.NoError(t, database.Conn().QueryRowContext(ctx, `SELECT COUNT(*) FROM feed_membership_claim WHERE item_id = ?`, item.ID).Scan(&claims))
	assert.Zero(t, items)
	assert.Zero(t, events)
	assert.Zero(t, claims)
}

func TestRunRetentionReclaimsOrphanedSourceHeads(t *testing.T) {
	database := openTestDB(t)
	ctx := t.Context()

	const topic = "source:flow/source"
	seedSourceHeadItem := func(externalID string) {
		t.Helper()
		_, err := database.InsertInboxItem(ctx, InsertInboxItemParams{
			ProfileID: "flow", SourceKind: "github", SourceScope: "scope", ExternalID: externalID,
			Title: externalID, Payload: []byte(`{"v":1}`), Lifecycle: "active", FirstSeenAt: 1, LastEventAt: 1,
		})
		require.NoError(t, err)
		require.NoError(t, database.UpsertSourceHead(ctx, UpsertSourceHeadParams{Topic: topic, Key: externalID, Payload: []byte(`{"v":1}`)}))
	}
	seedSourceHeadItem("expired")
	seedSourceHeadItem("live")
	_, err := database.Conn().ExecContext(ctx, `UPDATE inbox_item SET archived_at = ?, archived_actor = 'manual' WHERE external_id = 'expired'`, time.Now().Add(-91*24*time.Hour).UnixMilli())
	require.NoError(t, err)

	err = database.Prune(ctx, RetentionPolicy{ArchivedItemRetention: 90 * 24 * time.Hour})
	require.NoError(t, err)

	var expiredHeads, liveHeads int
	require.NoError(t, database.Conn().QueryRowContext(ctx, `SELECT COUNT(*) FROM source_head WHERE topic = ? AND key = 'expired'`, topic).Scan(&expiredHeads))
	require.NoError(t, database.Conn().QueryRowContext(ctx, `SELECT COUNT(*) FROM source_head WHERE topic = ? AND key = 'live'`, topic).Scan(&liveHeads))
	assert.Zero(t, expiredHeads, "the pruned item's orphaned head row is reclaimed")
	assert.Equal(t, 1, liveHeads, "a live item's head row survives retention")
}

func TestPrune_TrimsInboxEventsPerItem(t *testing.T) {
	database := openTestDB(t)
	ctx := t.Context()
	item := seedInboxItem(t, database, "flow", "many-events")
	other := seedInboxItem(t, database, "flow", "other-events")
	for i := range 5 {
		_, err := database.InsertInboxEvent(ctx, InsertInboxEventParams{
			ItemID: item.ID, Kind: "updated", Transition: "none", Attention: "activity", Detail: []byte(`{}`), CreatedAt: int64(i),
		})
		require.NoError(t, err)
	}
	for i := range 2 {
		_, err := database.InsertInboxEvent(ctx, InsertInboxEventParams{
			ItemID: other.ID, Kind: "updated", Transition: "none", Attention: "activity", Detail: []byte(`{}`), CreatedAt: int64(i),
		})
		require.NoError(t, err)
	}

	err := database.Prune(ctx, RetentionPolicy{EventPerItemLimit: 2})
	require.NoError(t, err)

	for _, want := range []struct {
		itemID int64
		ids    []int64
	}{
		{item.ID, []int64{4, 5}},
		{other.ID, []int64{6, 7}},
	} {
		var eventCount int
		require.NoError(t, database.Conn().QueryRowContext(ctx, `SELECT COUNT(*) FROM inbox_event WHERE item_id = ?`, want.itemID).Scan(&eventCount))
		assert.Equal(t, 2, eventCount)

		assert.Equal(t, want.ids, inboxEventIDs(t, ctx, database, want.itemID))
	}

	err = database.Prune(ctx, RetentionPolicy{EventPerItemLimit: 0})
	require.NoError(t, err)
	for _, itemID := range []int64{item.ID, other.ID} {
		var eventCount int
		require.NoError(t, database.Conn().QueryRowContext(ctx, `SELECT COUNT(*) FROM inbox_event WHERE item_id = ?`, itemID).Scan(&eventCount))
		assert.Zero(t, eventCount)
	}
}

func TestPrune_RejectsNegativeInboxRetentionLimits(t *testing.T) {
	database := openTestDB(t)
	err := database.Prune(t.Context(), RetentionPolicy{ArchivedItemRetention: -time.Hour})
	require.EqualError(t, err, "archived item retention must not be negative")
	err = database.Prune(t.Context(), RetentionPolicy{EventPerItemLimit: -1})
	require.EqualError(t, err, "event per-item retention limit must not be negative")
}

func TestOpen_FreshDB_HasRetentionIndexes(t *testing.T) {
	database := openTestDB(t)
	ctx := t.Context()

	for _, index := range []string{
		"idx_node_run_ended_at",
		"idx_node_run_flow_ended_at",
		"idx_output_command_terminal_id",
	} {
		var count int
		require.NoError(t, database.Conn().QueryRowContext(ctx,
			`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, index,
		).Scan(&count))
		assert.Equal(t, 1, count, "%s should exist", index)
	}
}

// seedInboxItem inserts one inbox_item row, the fixture Prune's retention
// tests build events, claims and archival state on top of.
func seedInboxItem(t *testing.T, db *DB, profile, external string) InboxItem {
	t.Helper()
	item, err := db.InsertInboxItem(t.Context(), InsertInboxItemParams{
		ProfileID: profile, SourceKind: "github", SourceScope: "scope", ExternalID: external,
		Payload: []byte(`{}`), Lifecycle: "active",
	})
	require.NoError(t, err)
	return item
}

// inboxEventIDs reads one item's surviving event ids. It is a function rather
// than an inline loop so rows.Close can be deferred per query instead of
// accumulating across the caller's iterations.
func inboxEventIDs(t *testing.T, ctx context.Context, database *DB, itemID int64) []int64 {
	t.Helper()
	rows, err := database.Conn().QueryContext(ctx, `SELECT id FROM inbox_event WHERE item_id = ? ORDER BY id`, itemID)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	var ids []int64
	for rows.Next() {
		var id int64
		require.NoError(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	require.NoError(t, rows.Err())
	return ids
}

func TestPrune_SweepsExpiredNodeKVAlways(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	past := time.Now().Add(-time.Minute).UnixMilli()
	future := time.Now().Add(time.Hour).UnixMilli()
	require.NoError(t, db.NodeKVSet(ctx, "flow", "fn", "expired", `1`, past))
	require.NoError(t, db.NodeKVSet(ctx, "flow", "fn", "live", `1`, future))
	require.NoError(t, db.NodeKVSet(ctx, "flow", "fn", "forever", `1`, 0))

	err := db.Prune(ctx, DefaultRetentionPolicy())
	require.NoError(t, err)

	var rows int
	require.NoError(t, db.Conn().QueryRowContext(ctx, `SELECT COUNT(*) FROM node_kv`).Scan(&rows))
	assert.Equal(t, 2, rows, "only the expired row is swept — pruning unexpired keys would re-notify their items")
}
