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

// fixtureFlowPath is desktop/e2e/fixtures/flows/frontend-triage.yaml,
// relative to this package's directory (go test's working directory) — the
// same fixture desktop/e2e/scripts/serve.sh points HIVE_DESKTOP_FLOWS at
// for the mock "feed" e2e server.
const fixtureFlowPath = "e2e/fixtures/flows/frontend-triage.yaml"

func TestFixtureFlow_LoadsAndMatchesSeedConstants(t *testing.T) {
	f, warnings, err := flow.LoadFlow(fixtureFlowPath, flow.MapRefs{})
	require.NoError(t, err)
	assert.Empty(t, warnings)

	assert.Equal(t, MockFlowID, f.ID, "fixture flow id must match MockFlowID")
	assert.Equal(t, "Frontend Triage", f.Name)
	assert.True(t, f.Enabled)

	var feedNode *flow.Node
	for i := range f.Nodes {
		if f.Nodes[i].Type == "feed" {
			feedNode = &f.Nodes[i]
		}
	}
	require.NotNil(t, feedNode, "fixture flow must have a feed node")
	assert.Equal(t, MockFeedNodeID, feedNode.ID, "fixture flow's feed node id must match MockFeedNodeID")
}

func TestSeedMockFeedItems_WritesExpectedRows(t *testing.T) {
	db, err := pipelinedb.Open(t.TempDir(), pipelinedb.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, seedMockFeedItems(db))

	items, err := db.FeedItems(context.Background(), mockFeedID())
	require.NoError(t, err)
	require.Len(t, items, len(mockFeedItems))

	// FeedItems orders newest-first (updated_at DESC); seedMockFeedItems
	// stamps a strictly decreasing updated_at per mockFeedItems index, so
	// the returned order must reproduce that slice's order exactly — this
	// is the invariant feed.spec.ts's item-order assertion depends on.
	gotIDs := make([]string, len(items))
	for i, it := range items {
		gotIDs[i] = it.ItemID
	}
	wantIDs := make([]string, len(mockFeedItems))
	for i, it := range mockFeedItems {
		wantIDs[i] = it.ID
	}
	assert.Equal(t, wantIDs, gotIDs)

	// Unread flags must round-trip, and the count of unread items must match
	// what feed.spec.ts's "filters the feed to its three unread items" test
	// expects.
	unreadCount := 0
	for i, it := range items {
		var payload feed.Item
		require.NoError(t, json.Unmarshal(it.Payload, &payload))
		assert.Equal(t, mockFeedItems[i].ID, payload.ID)
		assert.Equal(t, mockFeedItems[i].Title, payload.Title)
		assert.Equal(t, mockFeedItems[i].Unread, it.Unread)
		if it.Unread {
			unreadCount++
		}
	}
	assert.Equal(t, 3, unreadCount)

	counts, err := db.FeedItemCounts(context.Background(), MockFlowID)
	require.NoError(t, err)
	require.Len(t, counts, 1)
	assert.Equal(t, pipelinedb.FeedCount{FeedID: mockFeedID(), Total: 6, Unread: 3}, counts[0])
}

func TestSeedMockFeedItems_IdempotentOnRestart(t *testing.T) {
	db, err := pipelinedb.Open(t.TempDir(), pipelinedb.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, seedMockFeedItems(db))
	require.NoError(t, seedMockFeedItems(db)) // simulates a second process start

	items, err := db.FeedItems(context.Background(), mockFeedID())
	require.NoError(t, err)
	assert.Len(t, items, len(mockFeedItems), "re-seeding must upsert in place, not duplicate rows")
}
