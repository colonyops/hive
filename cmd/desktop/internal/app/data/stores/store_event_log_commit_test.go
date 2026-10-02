package stores

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
)

func TestCommit_FeedOutput_ClaimsResolvedInboxItem(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()
	item, err := db.InsertInboxItem(ctx, queries.InsertInboxItemParams{
		ProfileID: "flow-1", SourceKind: "github", SourceScope: "source-a", ExternalID: "item-1",
		Payload: []byte(`{"v":1}`), Lifecycle: "active",
	})
	require.NoError(t, err)

	require.NoError(t, st.EventLog.Commit(ctx, models.CommitBatch{
		Consumer: "flow-1", UpToOffset: 1,
		Outputs: []models.Output{{
			Sink: models.Sink{Kind: models.SinkKindFeed, TargetID: "feed-a"}, Key: "item-1",
			SourceKind: "github", SourceScope: "source-a", SourceTopic: "source:flow-1/source-a",
		}},
	}))

	var claims int
	require.NoError(t, db.Conn().QueryRowContext(ctx, `SELECT COUNT(*) FROM feed_membership_claim WHERE item_id = ? AND source_id = ?`, item.ID, "source:flow-1/source-a").Scan(&claims))
	assert.Equal(t, 1, claims)
}

// A row from before hay-kot/hive-desktop#63 carries an empty source_scope. A later
// feed output keys on the
// account scope, so the direct lookup misses; the commit must heal the row
// onto that scope, claim membership, and leave exactly one row — not wedge and
// not fork a duplicate. See issue hay-kot/hive-desktop#95.
func TestCommit_FeedOutput_HealsLegacyEmptyScopeItem(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()
	legacy, err := db.InsertInboxItem(ctx, queries.InsertInboxItemParams{
		ProfileID: "flow-1", SourceKind: "github", SourceScope: "", ExternalID: "colonyops/hive#199",
		Payload: []byte(`{"v":1}`), Lifecycle: "active", Unread: 1,
	})
	require.NoError(t, err)

	require.NoError(t, st.EventLog.Commit(ctx, models.CommitBatch{
		Consumer: "flow-1", UpToOffset: 1,
		Outputs: []models.Output{{
			Sink: models.Sink{Kind: models.SinkKindFeed, TargetID: "feed-a"}, Key: "colonyops/hive#199",
			SourceKind: "github", SourceScope: "hay-kot", SourceTopic: "source:flow-1/source-a",
		}},
	}))

	var scope string
	var rows int
	require.NoError(t, db.Conn().QueryRowContext(ctx, `SELECT source_scope FROM inbox_item WHERE id = ?`, legacy.ID).Scan(&scope))
	require.NoError(t, db.Conn().QueryRowContext(ctx, `SELECT COUNT(*) FROM inbox_item WHERE external_id = ?`, "colonyops/hive#199").Scan(&rows))
	assert.Equal(t, "hay-kot", scope, "the legacy row is rewritten to the current scope")
	assert.Equal(t, 1, rows, "healing rewrites the row rather than forking a duplicate")

	var claims int
	require.NoError(t, db.Conn().QueryRowContext(ctx, `SELECT COUNT(*) FROM feed_membership_claim WHERE item_id = ?`, legacy.ID).Scan(&claims))
	assert.Equal(t, 1, claims)
}

// A feed output whose key has no inbox row is one a function node synthesized
// while splitting a source message into per-entity items. The commit mints the
// row from the payload it carried and claims membership, so the item appears in
// the feed rather than being dropped.
func TestCommit_FeedOutput_MintsSynthesizedItem(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()

	require.NoError(t, st.EventLog.Commit(ctx, models.CommitBatch{
		Consumer: "flow-1", UpToOffset: 7,
		Outputs: []models.Output{{
			Sink: models.Sink{Kind: models.SinkKindFeed, TargetID: "feed-a"}, Key: "prod/flux/kustomization/apps",
			Payload:    json.RawMessage(`{"title":"apps ignored","cluster":"prod"}`),
			SourceKind: "grafana", SourceScope: "grafana/prod", SourceTopic: "source:flow-1/source-a",
		}},
	}))

	offset, err := st.EventLog.ConsumerOffset(ctx, "flow-1")
	require.NoError(t, err)
	assert.Equal(t, int64(7), offset, "the offset advances")

	var (
		title   string
		payload string
		unread  int
	)
	require.NoError(t, db.Conn().QueryRowContext(ctx,
		`SELECT title, payload, unread FROM inbox_item WHERE external_id = ?`, "prod/flux/kustomization/apps").
		Scan(&title, &payload, &unread))
	assert.Equal(t, "apps ignored", title, "the minted item takes its title from the payload")
	assert.JSONEq(t, `{"title":"apps ignored","cluster":"prod"}`, payload)
	assert.Equal(t, 1, unread, "a freshly synthesized item is unread")

	var claims int
	require.NoError(t, db.Conn().QueryRowContext(ctx, `SELECT COUNT(*) FROM feed_membership_claim`).Scan(&claims))
	assert.Equal(t, 1, claims, "the minted item claims feed membership")
}

func TestCommit_FeedOutput_MintedItemFallsBackToKeyForTitle(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()

	require.NoError(t, st.EventLog.Commit(ctx, models.CommitBatch{
		Consumer: "flow-1", UpToOffset: 1,
		Outputs: []models.Output{{
			Sink: models.Sink{Kind: models.SinkKindFeed, TargetID: "feed-a"}, Key: "prod/flux/kustomization/apps",
			Payload:    json.RawMessage(`{"cluster":"prod"}`),
			SourceKind: "grafana", SourceScope: "grafana/prod", SourceTopic: "source:flow-1/source-a",
		}},
	}))

	var title string
	require.NoError(t, db.Conn().QueryRowContext(ctx,
		`SELECT title FROM inbox_item WHERE external_id = ?`, "prod/flux/kustomization/apps").Scan(&title))
	assert.Equal(t, "prod/flux/kustomization/apps", title)
}

// A minted item's age is what the feed row renders, so `updatedAt` on the
// payload is honoured exactly as it is on a source's own message. Without this
// every per-entity item wore the mint time regardless of what the author put on
// it, and a payload built from a source that carries no time of its own (a
// Grafana metrics result) produced a row stamped at the epoch.
func TestCommit_FeedOutput_MintedItemTakesItsTimeFromThePayload(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()

	const stated = int64(1_600_000_000_000)
	require.NoError(t, st.EventLog.Commit(ctx, models.CommitBatch{
		Consumer: "flow-1", UpToOffset: 1,
		Outputs: []models.Output{{
			Sink: models.Sink{Kind: models.SinkKindFeed, TargetID: "feed-a"}, Key: "prod/flux/kustomization/apps",
			Payload:    json.RawMessage(`{"title":"apps","updatedAt":1600000000000}`),
			SourceKind: "grafana", SourceScope: "grafana/prod", SourceTopic: "source:flow-1/source-a",
		}},
	}))

	var lastEventAt, firstSeenAt int64
	require.NoError(t, db.Conn().QueryRowContext(ctx,
		`SELECT last_event_at, first_seen_at FROM inbox_item WHERE external_id = ?`, "prod/flux/kustomization/apps").
		Scan(&lastEventAt, &firstSeenAt))
	assert.Equal(t, stated, lastEventAt, "the item is as old as its payload says")
	assert.NotEqual(t, stated, firstSeenAt, "first_seen_at is when this app first saw it, not what the payload claims")
}

// A payload with no time of its own is stamped now. The alternative is a zero
// last_event_at, which renders as an item from 1970 rather than a new one.
func TestCommit_FeedOutput_MintedItemWithoutATimeIsStampedNow(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()

	before := time.Now().UnixMilli()
	require.NoError(t, st.EventLog.Commit(ctx, models.CommitBatch{
		Consumer: "flow-1", UpToOffset: 1,
		Outputs: []models.Output{{
			Sink: models.Sink{Kind: models.SinkKindFeed, TargetID: "feed-a"}, Key: "prod/flux/kustomization/apps",
			Payload:    json.RawMessage(`{"title":"apps"}`),
			SourceKind: "grafana", SourceScope: "grafana/prod", SourceTopic: "source:flow-1/source-a",
		}},
	}))

	var lastEventAt int64
	require.NoError(t, db.Conn().QueryRowContext(ctx,
		`SELECT last_event_at FROM inbox_item WHERE external_id = ?`, "prod/flux/kustomization/apps").Scan(&lastEventAt))
	assert.GreaterOrEqual(t, lastEventAt, before)
}

// A feed output with no key has no identity to mint under (the omitempty
// snapshot-boundary row of issue hay-kot/hive-desktop#95). It is skipped, not fatal: the offset has
// to advance so the consumer cannot be wedged at one such row forever.
func TestCommit_FeedOutput_SkipsKeylessItem(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()

	require.NoError(t, st.EventLog.Commit(ctx, models.CommitBatch{
		Consumer: "flow-1", UpToOffset: 7,
		Outputs: []models.Output{{
			Sink: models.Sink{Kind: models.SinkKindFeed, TargetID: "feed-a"}, Key: "",
			SourceKind: "github", SourceScope: "hay-kot", SourceTopic: "source:flow-1/source-a",
		}},
	}))

	offset, err := st.EventLog.ConsumerOffset(ctx, "flow-1")
	require.NoError(t, err)
	assert.Equal(t, int64(7), offset, "a keyless item is skipped, not fatal — the offset still advances")

	var (
		items  int
		claims int
	)
	require.NoError(t, db.Conn().QueryRowContext(ctx, `SELECT COUNT(*) FROM inbox_item`).Scan(&items))
	require.NoError(t, db.Conn().QueryRowContext(ctx, `SELECT COUNT(*) FROM feed_membership_claim`).Scan(&claims))
	assert.Zero(t, items, "nothing is minted for a keyless output")
	assert.Zero(t, claims)
}

func TestCommit_ActionOutput_EnqueuesOnce(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()

	countPending := func(t *testing.T) int {
		t.Helper()
		var count int
		require.NoError(t, db.Conn().QueryRowContext(
			ctx,
			`SELECT COUNT(*) FROM output_command WHERE action_id = ? AND key = ?`,
			"action-a", "item-1",
		).Scan(&count))
		return count
	}

	batch := models.CommitBatch{
		Consumer:   "flow-1",
		UpToOffset: 1,
		Outputs: []models.Output{
			{
				Sink:          models.Sink{Kind: models.SinkKindAction, TargetID: "action-a"},
				OccurrenceKey: "item-1",
				Payload:       []byte(`{"cmd":"do-it"}`),
			},
		},
	}
	require.NoError(t, st.EventLog.Commit(ctx, batch))
	assert.Equal(t, 1, countPending(t))

	// A duplicate (action_id, key) from a later batch is a no-op: the
	// action fires at most once.
	batch2 := models.CommitBatch{
		Consumer:   "flow-1",
		UpToOffset: 2,
		Outputs: []models.Output{
			{
				Sink:          models.Sink{Kind: models.SinkKindAction, TargetID: "action-a"},
				OccurrenceKey: "item-1",
				Payload:       []byte(`{"cmd":"do-it-again"}`),
			},
		},
	}
	require.NoError(t, st.EventLog.Commit(ctx, batch2))
	assert.Equal(t, 1, countPending(t), "duplicate (action_id, key) must not enqueue a second command")
}

func TestCommit_NotifyOutput_EnqueuesTheItemIdentityOnce(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()

	notifyOutput := func(occurrence string, payload string) models.Output {
		return models.Output{
			Sink:          models.Sink{Kind: models.SinkKindNotify, TargetID: "flow-1/tell-me"},
			Key:           "item-1",
			OccurrenceKey: occurrence,
			SourceKind:    "github",
			SourceScope:   "source-a",
			SourceTopic:   "source:flow-1/source-a",
			Payload:       []byte(payload),
		}
	}

	require.NoError(t, st.EventLog.Commit(ctx, models.CommitBatch{
		Consumer: "flow-1", UpToOffset: 1,
		Outputs: []models.Output{notifyOutput("item-1@2", `{"repo":"acme/api"}`)},
	}))

	var actionID, key string
	var payload []byte
	require.NoError(t, db.Conn().QueryRowContext(ctx,
		`SELECT action_id, key, payload FROM output_command`).Scan(&actionID, &key, &payload))
	assert.Equal(t, models.NotifyActionID("flow-1/tell-me"), actionID)
	assert.Equal(t, "item-1@2", key)

	var cmd models.NotifyCommand
	require.NoError(t, json.Unmarshal(payload, &cmd))
	assert.Equal(t, models.NotifyCommand{
		ProfileID:   "flow-1",
		ExternalID:  "item-1",
		SourceKind:  "github",
		SourceScope: "source-a",
		Item:        json.RawMessage(`{"repo":"acme/api"}`),
	}, cmd)

	// The same occurrence arriving again — a re-emitted, unchanged item —
	// must not interrupt the user a second time.
	require.NoError(t, st.EventLog.Commit(ctx, models.CommitBatch{
		Consumer: "flow-1", UpToOffset: 2,
		Outputs: []models.Output{notifyOutput("item-1@2", `{"repo":"acme/api"}`)},
	}))
	assert.Equal(t, 1, countOutputCommands(t, db, ctx))

	// A new occurrence for the same item is new information and enqueues.
	require.NoError(t, st.EventLog.Commit(ctx, models.CommitBatch{
		Consumer: "flow-1", UpToOffset: 3,
		Outputs: []models.Output{notifyOutput("item-1@3", `{"repo":"acme/api"}`)},
	}))
	assert.Equal(t, 2, countOutputCommands(t, db, ctx))
}

// Without an occurrence key the dedup key falls back to the payload digest,
// so an identical message still collapses while a changed one gets through —
// rather than the node notifying once and then going silent forever.
func TestCommit_NotifyOutput_DedupesOnPayloadWithoutAnOccurrenceKey(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()

	notifyOutput := func(payload string) models.Output {
		return models.Output{
			Sink:    models.Sink{Kind: models.SinkKindNotify, TargetID: "flow-1/tell-me"},
			Key:     "item-1",
			Payload: []byte(payload),
		}
	}

	require.NoError(t, st.EventLog.Commit(ctx, models.CommitBatch{
		Consumer: "flow-1", UpToOffset: 1, Outputs: []models.Output{notifyOutput(`{"v":1}`)},
	}))
	require.NoError(t, st.EventLog.Commit(ctx, models.CommitBatch{
		Consumer: "flow-1", UpToOffset: 2, Outputs: []models.Output{notifyOutput(`{"v":1}`)},
	}))
	assert.Equal(t, 1, countOutputCommands(t, db, ctx))

	require.NoError(t, st.EventLog.Commit(ctx, models.CommitBatch{
		Consumer: "flow-1", UpToOffset: 3, Outputs: []models.Output{notifyOutput(`{"v":2}`)},
	}))
	assert.Equal(t, 2, countOutputCommands(t, db, ctx))
}

func TestCommit_NotifyOutput_IsPerNode(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()

	require.NoError(t, st.EventLog.Commit(ctx, models.CommitBatch{
		Consumer: "flow-1", UpToOffset: 1,
		Outputs: []models.Output{
			{Sink: models.Sink{Kind: models.SinkKindNotify, TargetID: "flow-1/tell-me"}, Key: "item-1", OccurrenceKey: "occ", Payload: []byte(`{}`)},
			{Sink: models.Sink{Kind: models.SinkKindNotify, TargetID: "flow-1/also-tell-me"}, Key: "item-1", OccurrenceKey: "occ", Payload: []byte(`{}`)},
		},
	}))
	assert.Equal(t, 2, countOutputCommands(t, db, ctx))
}

func TestCommit_EmptySourceSnapshotClearsOnlyThatSourceClaims(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()
	item := seedReplayItem(t, db, "flow", "item")
	feed := models.Sink{Kind: models.SinkKindFeed, TargetID: "flow/feed"}
	commit := func(offset int64, source string) {
		snapshotID := strconv.FormatInt(offset, 10)
		require.NoError(t, st.EventLog.Commit(ctx, models.CommitBatch{
			Consumer: "flow", UpToOffset: offset,
			Outputs: []models.Output{{
				Sink: feed, Key: "item", SourceKind: "github", SourceScope: "scope", SourceTopic: source, SnapshotID: snapshotID,
			}},
			FeedSnapshots: []models.FeedSnapshot{{FeedID: feed.TargetID, SourceTopic: source, SnapshotID: snapshotID}},
		}))
	}
	commit(1, "source:flow/a")
	commit(2, "source:flow/b")
	require.NoError(t, st.EventLog.Commit(ctx, models.CommitBatch{
		Consumer: "flow", UpToOffset: 3,
		FeedSnapshots: []models.FeedSnapshot{{FeedID: feed.TargetID, SourceTopic: "source:flow/a", SnapshotID: "3"}},
	}))

	var source string
	require.NoError(t, db.Conn().QueryRowContext(ctx, `SELECT source_id FROM feed_membership_claim WHERE item_id = ?`, item.ID).Scan(&source))
	assert.Equal(t, "source:flow/b", source)
}

func countOutputCommands(t *testing.T, db *queries.DB, ctx context.Context) int {
	t.Helper()
	var count int
	require.NoError(t, db.Conn().QueryRowContext(ctx, `SELECT COUNT(*) FROM output_command`).Scan(&count))
	return count
}

func TestCommit_InsertsNodeRuns(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()

	batch := models.CommitBatch{
		Consumer:   "flow-1",
		UpToOffset: 1,
		NodeRuns: []models.NodeRun{
			{
				FlowID:    "flow-1",
				NodeID:    "node-a",
				OK:        true,
				InCount:   3,
				OutCount:  2,
				DropCount: 1,
				DurMs:     42,
			},
			{
				FlowID:   "flow-1",
				NodeID:   "node-b",
				OK:       false,
				Err:      "boom",
				InCount:  1,
				OutCount: 0,
			},
		},
	}
	require.NoError(t, st.EventLog.Commit(ctx, batch))

	rows, err := db.Conn().QueryContext(ctx,
		`SELECT node_id, ok, in_count, out_count, drop_count, err, dur_ms FROM node_run ORDER BY node_id`)
	require.NoError(t, err)
	defer func() { require.NoError(t, rows.Close()) }()

	type row struct {
		nodeID    string
		ok        int64
		inCount   int64
		outCount  int64
		dropCount int64
		err       *string
		durMs     int64
	}
	var got []row
	for rows.Next() {
		var r row
		require.NoError(t, rows.Scan(&r.nodeID, &r.ok, &r.inCount, &r.outCount, &r.dropCount, &r.err, &r.durMs))
		got = append(got, r)
	}
	require.NoError(t, rows.Err())
	require.Len(t, got, 2)

	assert.Equal(t, "node-a", got[0].nodeID)
	assert.Equal(t, int64(1), got[0].ok)
	assert.Equal(t, int64(3), got[0].inCount)
	assert.Equal(t, int64(2), got[0].outCount)
	assert.Equal(t, int64(1), got[0].dropCount)
	assert.Nil(t, got[0].err)
	assert.Equal(t, int64(42), got[0].durMs)

	assert.Equal(t, "node-b", got[1].nodeID)
	assert.Equal(t, int64(0), got[1].ok)
	require.NotNil(t, got[1].err)
	assert.Equal(t, "boom", *got[1].err)
}

func TestCommit_AdvancesOffset_AndIsIdempotentOnReplay(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()

	batch := models.CommitBatch{
		Consumer:   "flow-1",
		UpToOffset: 5,
		NodeRuns: []models.NodeRun{
			{FlowID: "flow-1", NodeID: "node-a", OK: true},
		},
	}
	require.NoError(t, st.EventLog.Commit(ctx, batch))

	offset, err := st.EventLog.ConsumerOffset(ctx, "flow-1")
	require.NoError(t, err)
	assert.Equal(t, int64(5), offset)

	countRows := func(t *testing.T, table string) int {
		t.Helper()
		var count int
		require.NoError(t, db.Conn().QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count))
		return count
	}
	nodeRunsBefore := countRows(t, "node_run")

	// Replaying the exact same batch (UpToOffset <= current) must be a
	// full no-op: no new node_run rows, offset unchanged.
	require.NoError(t, st.EventLog.Commit(ctx, batch))

	offset, err = st.EventLog.ConsumerOffset(ctx, "flow-1")
	require.NoError(t, err)
	assert.Equal(t, int64(5), offset, "replaying an already-applied batch must not change the offset")
	assert.Equal(t, nodeRunsBefore, countRows(t, "node_run"), "replay must not insert duplicate node_run rows")

	// A batch with UpToOffset below the current committed offset is also a
	// no-op, even with different/new outputs.
	staleBatch := models.CommitBatch{
		Consumer:   "flow-1",
		UpToOffset: 3,
		Outputs: []models.Output{{
			Sink:          models.Sink{Kind: models.SinkKindAction, TargetID: "action-a"},
			OccurrenceKey: "item-new",
			Payload:       []byte(`{"v":"new"}`),
		}},
	}
	require.NoError(t, st.EventLog.Commit(ctx, staleBatch))

	offset, err = st.EventLog.ConsumerOffset(ctx, "flow-1")
	require.NoError(t, err)
	assert.Equal(t, int64(5), offset, "a stale UpToOffset must not regress the committed offset")

	assert.Zero(t, countRows(t, "output_command"), "a no-op stale batch must not apply its outputs")
}

func TestCommit_UnknownSinkKind_Errors(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()

	batch := models.CommitBatch{
		Consumer:   "flow-1",
		UpToOffset: 1,
		Outputs: []models.Output{
			{Sink: models.Sink{Kind: "bogus", TargetID: "x"}, Key: "k", Payload: []byte(`{}`)},
		},
	}
	err := st.EventLog.Commit(ctx, batch)
	require.Error(t, err)

	// The whole batch must roll back: the offset must not advance either.
	offset, offsetErr := st.EventLog.ConsumerOffset(ctx, "flow-1")
	require.NoError(t, offsetErr)
	assert.Equal(t, int64(0), offset)
}

func TestCommit_KVMutations_FlushInTheSameTransaction(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()

	require.NoError(t, st.NodeKV.Set(ctx, "flow-1", "dedup", "stale", `1`, 0))
	require.NoError(t, st.EventLog.Commit(ctx, models.CommitBatch{
		Consumer: "flow-1", UpToOffset: 1,
		KVMutations: []models.KVMutation{
			{NodeID: "dedup", Key: "seen", Value: `true`, ExpiresAt: 9000},
			{NodeID: "dedup", Key: "stale", Delete: true},
		},
	}))

	value, found, err := st.NodeKV.Get(ctx, "flow-1", "dedup", "seen", 1000)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, `true`, value)

	_, found, err = st.NodeKV.Get(ctx, "flow-1", "dedup", "stale", 1000)
	require.NoError(t, err)
	assert.False(t, found)

	_, found, err = st.NodeKV.Get(ctx, "flow-1", "dedup", "seen", 9000)
	require.NoError(t, err)
	assert.False(t, found, "the flushed expiry is honored by reads")
}

func TestCommit_KVMutations_RollBackWithOutputsAndOffset(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()

	err := st.EventLog.Commit(ctx, models.CommitBatch{
		Consumer: "flow-1", UpToOffset: 1,
		Outputs: []models.Output{
			{Sink: models.Sink{Kind: models.SinkKindAction, TargetID: "action-a"}, OccurrenceKey: "item-1", Payload: []byte(`{}`)},
			{Sink: models.Sink{Kind: "bogus"}},
		},
		KVMutations: []models.KVMutation{{NodeID: "dedup", Key: "seen", Value: `true`}},
	})
	require.Error(t, err)

	var kvRows int
	require.NoError(t, db.Conn().QueryRowContext(ctx, `SELECT COUNT(*) FROM node_kv`).Scan(&kvRows))
	assert.Zero(t, kvRows)
	assert.Zero(t, countOutputCommands(t, db, ctx))
	_, err = db.GetConsumerOffset(ctx, "flow-1")
	require.Error(t, err, "the offset must not have advanced")
}

func TestCommit_KVMutations_SkippedByTheIdempotencyGuard(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()

	require.NoError(t, st.EventLog.Commit(ctx, models.CommitBatch{
		Consumer: "flow-1", UpToOffset: 2,
		KVMutations: []models.KVMutation{{NodeID: "dedup", Key: "seen", Value: `"first"`}},
	}))
	require.NoError(t, st.EventLog.Commit(ctx, models.CommitBatch{
		Consumer: "flow-1", UpToOffset: 2,
		KVMutations: []models.KVMutation{{NodeID: "dedup", Key: "seen", Value: `"replayed"`}},
	}))

	value, found, err := st.NodeKV.Get(ctx, "flow-1", "dedup", "seen", 1000)
	require.NoError(t, err)
	require.True(t, found)
	assert.JSONEq(t, `"first"`, value)
}

// A pull request that keeps changing reaches a launch node with a new
// occurrence each time; it must still launch once.
func TestCommit_LaunchOutput_DedupsOnTheItemAndCarriesItsRef(t *testing.T) {
	st, db := openTestStores(t)
	ctx := t.Context()

	launchOutput := func(occurrence, payload string) models.Output {
		return models.Output{
			Sink:          models.Sink{Kind: models.SinkKindLaunch, TargetID: "flow-1/review"},
			Key:           "acme/api#7",
			OccurrenceKey: occurrence,
			SourceKind:    "github",
			SourceScope:   "source-a",
			SourceTopic:   "source:flow-1/source-a",
			Payload:       []byte(payload),
		}
	}

	require.NoError(t, st.EventLog.Commit(ctx, models.CommitBatch{
		Consumer: "flow-1", UpToOffset: 1, Outputs: []models.Output{launchOutput("occ-1", `{"num":7}`)},
	}))
	require.NoError(t, st.EventLog.Commit(ctx, models.CommitBatch{
		Consumer: "flow-1", UpToOffset: 2, Outputs: []models.Output{launchOutput("occ-2", `{"num":7,"title":"edited"}`)},
	}))
	assert.Equal(t, 1, countOutputCommands(t, db, ctx))

	var actionID, key, profileID, sourceKind, sourceScope, externalID string
	var payload []byte
	require.NoError(t, db.Conn().QueryRowContext(ctx,
		`SELECT action_id, key, payload, profile_id, source_kind, source_scope, external_id FROM output_command`,
	).Scan(&actionID, &key, &payload, &profileID, &sourceKind, &sourceScope, &externalID))
	assert.Equal(t, models.LaunchActionID("flow-1/review"), actionID)
	assert.Equal(t, "acme/api#7", key)
	assert.JSONEq(t, `{"num":7}`, string(payload))
	assert.Equal(t, []string{"flow-1", "github", "source-a", "acme/api#7"}, []string{profileID, sourceKind, sourceScope, externalID})

	require.NoError(t, st.EventLog.Commit(ctx, models.CommitBatch{
		Consumer: "flow-1", UpToOffset: 3, Outputs: []models.Output{{
			Sink: models.Sink{Kind: models.SinkKindLaunch, TargetID: "flow-1/also-review"}, Key: "acme/api#7", Payload: []byte(`{}`),
		}},
	}))
	assert.Equal(t, 2, countOutputCommands(t, db, ctx), "each launch node launches once for the item")
}
