package app

import (
	"testing"

	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
	ghsource "github.com/colonyops/hive/cmd/desktop/internal/app/sources/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStarterSeedFetchesAsTheGivenAccount(t *testing.T) {
	seed := starterSeed(seedRef)

	sources := 0
	for _, node := range seed.Nodes {
		if node.Type != ghsource.Descriptor.Type {
			continue
		}
		sources++
		cfg, ok := node.Config.(*flow.SourceConfig)
		require.True(t, ok, "node %q should carry a source config", node.ID)
		connector, ok := cfg.Connector().(*ghsource.Config)
		require.True(t, ok, "node %q should carry a github connector config", node.ID)
		assert.Equal(t, seedRef, connector.Credential)
	}
	assert.Positive(t, sources, "the starter graph is made of source nodes")
}

func TestStarterSeedShipsAReviewRequestsNotifyBranch(t *testing.T) {
	seed := starterSeed(seedRef)

	nodesByID := map[string]flow.Node{}
	for _, node := range seed.Nodes {
		nodesByID[node.ID] = node
	}

	filter, ok := nodesByID["review-requests-filter"]
	require.True(t, ok, "the starter graph should ship a review-request filter")
	filterCfg, ok := filter.Config.(*flow.GithubFilterConfig)
	require.True(t, ok, "node %q should carry a github filter config", filter.ID)
	require.Equal(t, []string{"review_requested"}, filterCfg.Reasons)

	reviewRequests, ok := nodesByID["review-requests"]
	require.True(t, ok, "the starter graph should ship a review-requests feed")
	_, ok = reviewRequests.Config.(*flow.FeedConfig)
	require.True(t, ok, "node %q should carry a feed config", reviewRequests.ID)

	// The interrupt is a notify node on its own branch — a feed never
	// notifies, so "tell me when I'm asked to review" ships as its own
	// terminal beside the feed that collects the same items.
	notifyNode, ok := nodesByID["review-requests-notify"]
	require.True(t, ok, "the starter graph should ship a review-requests notify node")
	notify, ok := notifyNode.Config.(*flow.NotifyConfig)
	require.True(t, ok, "node %q should carry a notify config", notifyNode.ID)
	require.NotEmpty(t, notify.Title)

	// The filter branches off the notifications source rather than sitting in
	// front of the Notifications feed: everything still lands there too.
	assert.Contains(t, seed.Wires, flow.Wire{From: "notifications-src", To: "notifications"})
	assert.Contains(t, seed.Wires, flow.Wire{From: "notifications-src", To: "review-requests-filter"})
	assert.Contains(t, seed.Wires, flow.Wire{From: "review-requests-filter", Out: 0, To: "review-requests"})
	assert.Contains(t, seed.Wires, flow.Wire{From: "review-requests-filter", Out: 0, To: "review-requests-notify"})
	assert.Contains(t, seed.Layout.Nodes, "review-requests", "the seeded node needs a canvas position")
	assert.Contains(t, seed.Layout.Nodes, "review-requests-notify", "the seeded node needs a canvas position")
}
