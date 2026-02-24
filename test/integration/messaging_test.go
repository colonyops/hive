//go:build integration

package integration

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMsgPubSub(t *testing.T) {
	h := NewHarness(t)

	// Publish a message
	_, err := h.Run("msg", "pub", "--topic", "test.pubsub", "--sender", "test", "hello world")
	require.NoError(t, err)

	// Subscribe and read it back
	out, err := h.Run("msg", "sub", "--topic", "test.pubsub")
	require.NoError(t, err, "msg sub: %s", out)

	lines, err := parseJSONLines(strings.TrimSpace(out))
	require.NoError(t, err)
	require.Len(t, lines, 1)
	assert.Equal(t, "hello world", lines[0]["payload"])
}

func TestMsgList(t *testing.T) {
	h := NewHarness(t)

	// Publish to two topics
	_, err := h.Run("msg", "pub", "--topic", "test.list-a", "--sender", "test", "msg-a")
	require.NoError(t, err)
	_, err = h.Run("msg", "pub", "--topic", "test.list-b", "--sender", "test", "msg-b")
	require.NoError(t, err)

	// List topics
	out, err := h.Run("msg", "list")
	require.NoError(t, err, "msg list: %s", out)

	lines, err := parseJSONLines(strings.TrimSpace(out))
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(lines), 2)

	// Verify both topics appear
	names := make([]string, len(lines))
	for i, l := range lines {
		names[i] = l["name"].(string)
	}
	assert.Contains(t, names, "test.list-a")
	assert.Contains(t, names, "test.list-b")
}

func TestMsgTopic(t *testing.T) {
	h := NewHarness(t)

	out, err := h.Run("msg", "topic")
	require.NoError(t, err, "msg topic: %s", out)
	topic := strings.TrimSpace(out)
	assert.NotEmpty(t, topic)
	// Default prefix is "agent"
	assert.True(t, strings.HasPrefix(topic, "agent."), "topic should start with agent.: %s", topic)
}

func TestMsgSubEmpty(t *testing.T) {
	h := NewHarness(t)

	// Subscribe to nonexistent topic should return empty (no error)
	out, err := h.Run("msg", "sub", "--topic", "nonexistent.topic.xyz")
	require.NoError(t, err, "msg sub empty: %s", out)
	assert.Empty(t, strings.TrimSpace(out))
}

func TestMsgInbox(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "inbox-repo")

	// Create a session to get a session ID
	_, err := h.Run("new", "--remote", repo, "inbox-test")
	require.NoError(t, err)

	// Get session info to find the inbox topic
	out, err := h.Run("ls", "--json")
	require.NoError(t, err)

	lines, err := parseJSONLines(strings.TrimSpace(out))
	require.NoError(t, err)
	require.Len(t, lines, 1)

	inboxTopic := lines[0]["inbox"].(string)
	require.NotEmpty(t, inboxTopic)

	// Publish a message to the session's inbox
	_, err = h.Run("msg", "pub", "--topic", inboxTopic, "--sender", "test", "inbox message")
	require.NoError(t, err)

	// Read from inbox (need to be in session dir for auto-detect)
	// Since we can't easily be in the session dir, verify via sub instead
	subOut, err := h.Run("msg", "sub", "--topic", inboxTopic)
	require.NoError(t, err, "sub inbox: %s", subOut)

	subLines, err := parseJSONLines(strings.TrimSpace(subOut))
	require.NoError(t, err)
	require.Len(t, subLines, 1)
	assert.Equal(t, "inbox message", subLines[0]["payload"])
}
