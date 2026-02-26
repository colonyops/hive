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

	_, err := h.Run("msg", "pub", "--topic", "test.pubsub", "--sender", "test", "hello world")
	require.NoError(t, err)

	lines, err := h.RunJSONLines("msg", "sub", "--topic", "test.pubsub")
	require.NoError(t, err)
	require.Len(t, lines, 1)
	assert.Equal(t, "hello world", lines[0]["payload"])
}

func TestMsgPubConfirmation(t *testing.T) {
	h := NewHarness(t)

	// pub now returns JSON confirmation on stdout
	lines, err := h.RunJSONLines("msg", "pub", "--topic", "test.confirm", "--sender", "tester", "-m", "hello")
	require.NoError(t, err)
	require.Len(t, lines, 1)

	assert.Equal(t, "ok", lines[0]["status"])
	topics, ok := lines[0]["topics"].([]any)
	require.True(t, ok, "topics should be an array")
	assert.Contains(t, topics, "test.confirm")
	assert.Equal(t, "tester", lines[0]["sender"])
}

func TestMsgList(t *testing.T) {
	h := NewHarness(t)

	_, err := h.Run("msg", "pub", "--topic", "test.list-a", "--sender", "test", "msg-a")
	require.NoError(t, err)
	_, err = h.Run("msg", "pub", "--topic", "test.list-b", "--sender", "test", "msg-b")
	require.NoError(t, err)

	lines, err := h.RunJSONLines("msg", "list")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(lines), 2)

	names := make([]string, len(lines))
	for i, l := range lines {
		name, ok := l["name"].(string)
		require.True(t, ok, "msg list entry missing 'name' string field: %v", l)
		names[i] = name
	}
	assert.Contains(t, names, "test.list-a")
	assert.Contains(t, names, "test.list-b")
}

func TestMsgListEnriched(t *testing.T) {
	h := NewHarness(t)

	_, err := h.Run("msg", "pub", "--topic", "test.enriched", "--sender", "enricher", "payload")
	require.NoError(t, err)

	lines, err := h.RunJSONLines("msg", "list")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(lines), 1)

	// Find the enriched topic
	var found map[string]any
	for _, l := range lines {
		if l["name"] == "test.enriched" {
			found = l
			break
		}
	}
	require.NotNil(t, found, "test.enriched topic not found in list output")

	assert.Equal(t, float64(1), found["message_count"])
	assert.NotEmpty(t, found["last_activity"], "should have last_activity")
	assert.Equal(t, "enricher", found["last_sender"])
}

func TestMsgTopic(t *testing.T) {
	h := NewHarness(t)

	out, err := h.RunStdout("msg", "topic")
	require.NoError(t, err, "msg topic: %s", out)
	topic := strings.TrimSpace(out)
	assert.NotEmpty(t, topic)
	assert.True(t, strings.HasPrefix(topic, "agent."), "topic should start with agent.: %s", topic)
}

func TestMsgSubEmpty(t *testing.T) {
	h := NewHarness(t)

	lines, err := h.RunJSONLines("msg", "sub", "--topic", "nonexistent.topic.xyz")
	require.NoError(t, err)
	assert.Empty(t, lines)
}

func TestMsgInbox(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "inbox-repo")

	_, err := h.Run("new", "--remote", repo, "inbox-test")
	require.NoError(t, err)

	lines, err := h.RunJSONLines("ls", "--json")
	require.NoError(t, err)
	require.Len(t, lines, 1)

	inboxTopic, ok := lines[0]["inbox"].(string)
	require.True(t, ok, "session missing 'inbox' string field: %v", lines[0])
	require.NotEmpty(t, inboxTopic)

	_, err = h.Run("msg", "pub", "--topic", inboxTopic, "--sender", "test", "inbox message")
	require.NoError(t, err)

	subLines, err := h.RunJSONLines("msg", "sub", "--topic", inboxTopic)
	require.NoError(t, err)
	require.Len(t, subLines, 1)
	assert.Equal(t, "inbox message", subLines[0]["payload"])
}
