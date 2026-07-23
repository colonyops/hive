package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/desktop/feed"
	"github.com/colonyops/hive/internal/desktop/pipeline/flow"
	"github.com/colonyops/hive/internal/desktop/pipeline/pipelinedb"
)

const fixtureFlowPath = "e2e/fixtures/flows/frontend-triage.yaml"

type testFlowRefs struct {
	actions map[string]bool
}

func (r testFlowRefs) ResolveAction(id string) bool {
	return r.actions[id]
}

func TestFixtureFlow_LoadsAndMatchesSeedConstants(t *testing.T) {
	f, warnings, err := flow.LoadFlow(fixtureFlowPath, testFlowRefs{})
	require.NoError(t, err)
	assert.Empty(t, warnings)

	assert.Equal(t, MockFlowID, f.ID, "fixture flow id must match MockFlowID")
	assert.Equal(t, "Frontend Triage", f.Name)
	assert.True(t, f.Enabled)

	var feedNode, sourceNode *flow.Node
	for i := range f.Nodes {
		switch f.Nodes[i].Type {
		case "feed":
			feedNode = &f.Nodes[i]
		case "github-source":
			sourceNode = &f.Nodes[i]
		}
	}
	require.NotNil(t, sourceNode, "fixture flow must have a GitHub source node")
	assert.Equal(t, MockSourceNodeID, sourceNode.ID, "fixture flow's source node id must match MockSourceNodeID")
	require.NotNil(t, feedNode, "fixture flow must have a feed node")
	assert.Equal(t, MockFeedNodeID, feedNode.ID, "fixture flow's feed node id must match MockFeedNodeID")
}

func TestSeedMockInboxItems_WritesExpectedRows(t *testing.T) {
	db, err := pipelinedb.Open(t.TempDir(), pipelinedb.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, seedMockInboxItems(db))

	rows, err := db.Conn().QueryContext(context.Background(), `
		SELECT external_id, source_kind, source_scope, payload, unread, last_event_at
		FROM inbox_item
		WHERE profile_id = ?
		ORDER BY last_event_at DESC`, MockFlowID)
	require.NoError(t, err)
	defer func() { require.NoError(t, rows.Close()) }()

	for i, want := range mockInboxItems {
		require.True(t, rows.Next(), "missing seeded row %d", i)
		var (
			externalID  string
			sourceKind  string
			sourceScope string
			payload     []byte
			unread      int64
			lastEventAt int64
		)
		require.NoError(t, rows.Scan(&externalID, &sourceKind, &sourceScope, &payload, &unread, &lastEventAt))
		assert.Equal(t, want.ID, externalID)
		assert.Equal(t, "github", sourceKind)
		assert.Empty(t, sourceScope)
		assert.Positive(t, lastEventAt)
		var got feed.Item
		require.NoError(t, json.Unmarshal(payload, &got))
		assert.Equal(t, want.ID, got.ID)
		assert.Equal(t, want.Title, got.Title)
		assert.Equal(t, want.Unread, unread == 1)
	}
	assert.False(t, rows.Next())
	require.NoError(t, rows.Err())

	visible, err := db.ListInboxItems(context.Background(), MockFlowID, "inbox", len(mockInboxItems))
	require.NoError(t, err)
	assert.Len(t, visible, len(mockInboxItems))

	tail, err := db.EventLogTailOffset(context.Background())
	require.NoError(t, err)
	snapshots, err := db.ListReplaySourceSnapshots(context.Background(), MockFlowID, tail)
	require.NoError(t, err)
	require.Len(t, snapshots, 1)
	assert.Equal(t, "source:"+MockFlowID+"/"+MockSourceNodeID, snapshots[0].Topic)
	assert.Len(t, snapshots[0].Snapshot, len(mockInboxItems))
}
