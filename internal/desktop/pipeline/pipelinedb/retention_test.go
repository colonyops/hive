package pipelinedb

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrune_EventLogIsNotGatedByConsumerOffsets(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	for i := range 5 {
		_, err := database.Append(ctx, "source:test", fmt.Sprintf("key-%d", i), []byte(`{}`))
		require.NoError(t, err)
	}
	require.NoError(t, database.CommitBatch(ctx, CommitBatch{Consumer: "fast-flow", UpToOffset: "5"}))
	require.NoError(t, database.CommitBatch(ctx, CommitBatch{Consumer: "slow-flow", UpToOffset: "2"}))

	result, err := database.Prune(ctx, []string{"fast-flow", "slow-flow"}, DefaultRetentionPolicy())
	require.NoError(t, err)
	assert.Zero(t, result.EventLogThrough)

	msgs, _, err := database.ReadFrom(ctx, 0, 10)
	require.NoError(t, err)
	require.Len(t, msgs, 5)
	assert.Equal(t, []string{"1", "2", "3", "4", "5"}, []string{msgs[0].ID, msgs[1].ID, msgs[2].ID, msgs[3].ID, msgs[4].ID})
}

func TestPrune_EventLogUsesAgeWithoutConsumers(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	_, err := database.Append(ctx, "source:test", "old", []byte(`{}`))
	require.NoError(t, err)
	_, err = database.Append(ctx, "source:test", "new", []byte(`{}`))
	require.NoError(t, err)
	_, err = database.Conn().ExecContext(ctx, `UPDATE event_log SET created_at = ? WHERE "offset" = 1`, time.Now().Add(-2*time.Hour).UnixMilli())
	require.NoError(t, err)
	_, err = database.Conn().ExecContext(ctx, `UPDATE event_log SET created_at = ? WHERE "offset" = 2`, time.Now().UnixMilli())
	require.NoError(t, err)

	_, err = database.Prune(ctx, nil, RetentionPolicy{EventLogMaxAge: time.Hour})
	require.NoError(t, err)
	msgs, _, err := database.ReadFrom(ctx, 0, 10)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "2", msgs[0].ID)
}

func TestPrune_EventLogUsesPerTopicCountWithoutConsumers(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	for i := range 3 {
		_, err := database.Append(ctx, "source:test", fmt.Sprintf("key-%d", i), []byte(`{}`))
		require.NoError(t, err)
	}
	require.NoError(t, database.CommitBatch(ctx, CommitBatch{Consumer: "committed", UpToOffset: "3"}))

	_, err := database.Prune(ctx, []string{"committed", "newly-enabled"}, RetentionPolicy{EventLogPerTopicLimit: 1})
	require.NoError(t, err)

	msgs, _, err := database.ReadFrom(ctx, 0, 10)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "3", msgs[0].ID)
}

func TestPrune_BoundsOnlyTerminalHistory(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

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
		_, err := database.InsertJob(ctx, JobRecord{
			CreatedAt: int64(i), UpdatedAt: int64(i), Status: status, Label: fmt.Sprintf("job-%d", i),
		})
		require.NoError(t, err)
	}

	_, err := database.Prune(ctx, nil, RetentionPolicy{
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

	jobs, err := database.ListJobs(ctx, 0, 10)
	require.NoError(t, err)
	require.Len(t, jobs, 4)
	assert.Equal(t, []string{"running", "queued", "done", "failed"}, []string{
		jobs[0].Status, jobs[1].Status, jobs[2].Status, jobs[3].Status,
	})
}

func TestPrune_RejectsNegativeJobLimit(t *testing.T) {
	database := openTestDB(t)
	_, err := database.Prune(context.Background(), nil, RetentionPolicy{JobLimit: -1})
	require.EqualError(t, err, "job retention limit must not be negative")
}

func TestPrune_PrunesExpiredArchivedInboxItemsAndCascades(t *testing.T) {
	database := openTestDB(t)
	ctx := t.Context()
	item := seedReplayItem(t, database, "flow", "expired")
	_, err := database.Queries().InsertInboxEvent(ctx, InsertInboxEventParams{
		ItemID: item.ID, Kind: "updated", Transition: "none", Attention: "activity", Detail: []byte(`{}`), CreatedAt: 1,
	})
	require.NoError(t, err)
	require.NoError(t, database.Queries().UpsertFeedMembershipClaim(ctx, UpsertFeedMembershipClaimParams{
		ProfileID: "flow", FeedID: "flow/feed", ItemID: item.ID, SourceID: "source:flow/source",
	}))
	_, err = database.Conn().ExecContext(ctx, `UPDATE inbox_item SET archived_at = ?, archived_actor = 'manual' WHERE id = ?`, time.Now().Add(-91*24*time.Hour).UnixMilli(), item.ID)
	require.NoError(t, err)

	_, err = database.Prune(ctx, nil, RetentionPolicy{ArchivedItemRetention: 90 * 24 * time.Hour})
	require.NoError(t, err)

	var items, events, claims int
	require.NoError(t, database.Conn().QueryRowContext(ctx, `SELECT COUNT(*) FROM inbox_item WHERE id = ?`, item.ID).Scan(&items))
	require.NoError(t, database.Conn().QueryRowContext(ctx, `SELECT COUNT(*) FROM inbox_event WHERE item_id = ?`, item.ID).Scan(&events))
	require.NoError(t, database.Conn().QueryRowContext(ctx, `SELECT COUNT(*) FROM feed_membership_claim WHERE item_id = ?`, item.ID).Scan(&claims))
	assert.Zero(t, items)
	assert.Zero(t, events)
	assert.Zero(t, claims)
}

func TestPrune_TrimsInboxEventsPerItem(t *testing.T) {
	database := openTestDB(t)
	ctx := t.Context()
	item := seedReplayItem(t, database, "flow", "many-events")
	other := seedReplayItem(t, database, "flow", "other-events")
	for i := range 5 {
		_, err := database.Queries().InsertInboxEvent(ctx, InsertInboxEventParams{
			ItemID: item.ID, Kind: "updated", Transition: "none", Attention: "activity", Detail: []byte(`{}`), CreatedAt: int64(i),
		})
		require.NoError(t, err)
	}
	for i := range 2 {
		_, err := database.Queries().InsertInboxEvent(ctx, InsertInboxEventParams{
			ItemID: other.ID, Kind: "updated", Transition: "none", Attention: "activity", Detail: []byte(`{}`), CreatedAt: int64(i),
		})
		require.NoError(t, err)
	}

	_, err := database.Prune(ctx, nil, RetentionPolicy{EventPerItemLimit: 2})
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

		rows, err := database.Conn().QueryContext(ctx, `SELECT id FROM inbox_event WHERE item_id = ? ORDER BY id`, want.itemID)
		require.NoError(t, err)
		var ids []int64
		for rows.Next() {
			var id int64
			require.NoError(t, rows.Scan(&id))
			ids = append(ids, id)
		}
		require.NoError(t, rows.Close())
		require.NoError(t, rows.Err())
		assert.Equal(t, want.ids, ids)
	}

	_, err = database.Prune(ctx, nil, RetentionPolicy{EventPerItemLimit: 0})
	require.NoError(t, err)
	for _, itemID := range []int64{item.ID, other.ID} {
		var eventCount int
		require.NoError(t, database.Conn().QueryRowContext(ctx, `SELECT COUNT(*) FROM inbox_event WHERE item_id = ?`, itemID).Scan(&eventCount))
		assert.Zero(t, eventCount)
	}
}

func TestPrune_RejectsNegativeInboxRetentionLimits(t *testing.T) {
	database := openTestDB(t)
	_, err := database.Prune(t.Context(), nil, RetentionPolicy{ArchivedItemRetention: -time.Hour})
	require.EqualError(t, err, "archived item retention must not be negative")
	_, err = database.Prune(t.Context(), nil, RetentionPolicy{EventPerItemLimit: -1})
	require.EqualError(t, err, "event per-item retention limit must not be negative")
}

func TestOpen_FreshDB_HasRetentionIndexes(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

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
