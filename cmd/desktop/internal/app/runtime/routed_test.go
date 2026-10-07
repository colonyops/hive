package runtime

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
	whsource "github.com/colonyops/hive/cmd/desktop/internal/app/sources/webhook"
)

func webhookNode(id string) flow.Node {
	return flow.Node{ID: id, Type: "sources.webhook", Config: flow.NewSourceConfig(whsource.Descriptor.Type, &whsource.Config{Path: id})}
}

// A topic the log read leaves out must be one no entry would have accepted,
// or the flow would silently lose messages.
func TestRoutedTopicsMatchesEntryAcceptance(t *testing.T) {
	runner, err := NewRunner(flow.Flow{
		ID:    "f",
		Nodes: []flow.Node{webhookNode("a"), webhookNode("b"), {ID: "inbox", Type: "feed", Config: &flow.FeedConfig{}}},
		Wires: []flow.Wire{{From: "a", To: "inbox"}, {From: "b", To: "inbox"}},
	}, Options{})
	require.NoError(t, err)

	routed := runner.routedTopics()
	assert.False(t, routed.All)
	for _, topic := range []string{"source:f/a", "source:f/b", "source:other/a", "source:f/inbox"} {
		accepted := false
		for _, id := range runner.graph.Entries() {
			accepted = accepted || runner.acceptsEntry(runner.graph.Node(id), models.Msg{Topic: topic})
		}
		assert.Equal(t, accepted, routed.Has(topic), topic)
	}
}

func TestRoutedTopicsCoversEverythingForANonSourceEntry(t *testing.T) {
	runner, err := NewRunner(flow.Flow{
		ID:    "f",
		Nodes: []flow.Node{webhookNode("a"), {ID: "inbox", Type: "feed", Config: &flow.FeedConfig{}}},
	}, Options{})
	require.NoError(t, err)

	assert.True(t, runner.routedTopics().All)
}
