package flow

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/webhook"
)

func webhookFlow() Flow {
	return Flow{
		ID: "hooks", Name: "Hooks", Enabled: true,
		Nodes: []Node{
			{ID: "hook", Type: "sources.webhook", Config: NewSourceConfig(webhook.Descriptor.Type, &webhook.Config{Path: "ci"})},
			{ID: "inbox", Type: "feed", Name: "Inbox", Config: &FeedConfig{}},
		},
		Wires: []Wire{{From: "hook", To: "inbox"}},
	}
}

func TestSetSourceImageRoundTrips(t *testing.T) {
	store := NewFlowStore(t.TempDir(), minimalRefs())
	require.NoError(t, store.Save(webhookFlow()))

	ref, err := store.SourceImageRef("hooks", "hook")
	require.NoError(t, err)
	assert.Empty(t, ref, "a fresh webhook node has no mark image")

	hash := "0123456789abcdef0123456789abcdef"
	if _, err := store.SetSourceImage("hooks", "hook", hash); err != nil {
		require.NoError(t, err)
	}
	ref, err = store.SourceImageRef("hooks", "hook")
	require.NoError(t, err)
	assert.Equal(t, hash, ref, "the reference persists through the YAML round-trip")

	if _, err := store.SetSourceImage("hooks", "hook", ""); err != nil {
		require.NoError(t, err)
	}
	ref, err = store.SourceImageRef("hooks", "hook")
	require.NoError(t, err)
	assert.Empty(t, ref, "clearing reverts the node to its glyph")
}

func TestSetSourceImageErrors(t *testing.T) {
	store := NewFlowStore(t.TempDir(), minimalRefs())
	require.NoError(t, store.Save(webhookFlow()))

	_, err := store.SetSourceImage("nope", "hook", "0123456789abcdef0123456789abcdef")
	require.ErrorIs(t, err, ErrFlowNotFound)

	_, err = store.SetSourceImage("hooks", "ghost", "0123456789abcdef0123456789abcdef")
	require.ErrorIs(t, err, ErrNodeNotFound)

	// The feed terminal is not a source node, so it carries no image mark.
	_, err = store.SetSourceImage("hooks", "inbox", "0123456789abcdef0123456789abcdef")
	require.ErrorIs(t, err, ErrNodeNotImageMarkable)

	// A traversal-shaped flow id can never resolve to a file.
	_, err = store.SetSourceImage("../etc/passwd", "hook", "0123456789abcdef0123456789abcdef")
	require.ErrorIs(t, err, ErrFlowNotFound)
}
