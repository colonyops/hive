package runtime

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
)

// The two registries are the two halves of one declaration: flow.registry
// says how a node type is configured, and behaviors says what it does at run
// time. A type in one and not the other is a node the editor offers and the
// engine cannot run, or the reverse — and both fail at the worst moment
// (deploy, or the first message), so they fail here instead.
func TestEveryNodeTypeHasARuntimeBehavior(t *testing.T) {
	t.Parallel()

	configured := flow.NodeTypes()
	require.NotEmpty(t, configured)

	for _, nodeType := range configured {
		behavior, ok := behaviors[nodeType]
		require.Truef(t, ok, "node type %q is registered in flow but the engine has no behavior for it", nodeType)

		declared := 0
		if behavior.relay {
			declared++
		}
		if behavior.sinks != nil {
			declared++
		}
		if behavior.processor != nil {
			declared++
		}
		require.Equalf(t, 1, declared, "node type %q must declare exactly one runtime behavior", nodeType)
	}

	for nodeType := range behaviors {
		require.Containsf(t, configured, nodeType, "the engine has a behavior for %q but no node type is registered under it", nodeType)
	}
}

// A terminal's outputs are one message's committed side effects. The counts
// here are the contract the engine's accounting depends on.
func TestTerminalSinks(t *testing.T) {
	t.Parallel()

	msg := models.Msg{ID: "1", Payload: json.RawMessage(`{"title":"hi"}`)}
	msg.Key = "k"
	msg.OccurrenceKey = "oc"
	msg.Topic = "source:f/src"
	msg.SourceKind = "github"
	msg.SourceScope = "notifications"

	t.Run("a feed claims membership and nothing else", func(t *testing.T) {
		t.Parallel()
		outputs := feedSinks("f", "inbox", &flow.FeedConfig{}, msg)
		require.Len(t, outputs, 1)
		require.Equal(t, "feed", outputs[0].Sink.Kind)
		require.Equal(t, "f/inbox", outputs[0].Sink.TargetID)
		require.Equal(t, "k", outputs[0].Key)
		require.JSONEq(t, `{"title":"hi"}`, string(outputs[0].Payload), "the payload rides along so the commit can mint an inbox row for a key the producer never ingested")
	})

	t.Run("an action names its catalog id and carries no source identity", func(t *testing.T) {
		t.Parallel()
		outputs := actionSinks("f", "run", &flow.ActionConfig{Action: "triage-it"}, msg)
		require.Len(t, outputs, 1)
		require.Equal(t, "action", outputs[0].Sink.Kind)
		require.Equal(t, "triage-it", outputs[0].Sink.TargetID)
		require.Equal(t, "oc", outputs[0].OccurrenceKey, "the occurrence key is the dedup key that stops an action re-firing")
		require.Empty(t, outputs[0].SourceTopic)
	})

	t.Run("a notify node carries source identity so the banner can reveal the item", func(t *testing.T) {
		t.Parallel()
		outputs := notifySinks("f", "ping", &flow.NotifyConfig{Title: "Ping"}, msg)
		require.Len(t, outputs, 1)
		require.Equal(t, "notify", outputs[0].Sink.Kind)
		require.Equal(t, "f/ping", outputs[0].Sink.TargetID)
		require.Equal(t, "github", outputs[0].SourceKind)
		require.Equal(t, "source:f/src", outputs[0].SourceTopic)
	})
}
