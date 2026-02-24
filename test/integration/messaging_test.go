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

	out, err := h.RunStdout("msg", "sub", "--topic", "test.pubsub")
	require.NoError(t, err, "msg sub: %s", out)

	lines, err := parseJSONLines(strings.TrimSpace(out))
	require.NoError(t, err)
	require.Len(t, lines, 1)
	assert.Equal(t, "hello world", lines[0]["payload"])
}

func TestMsgList(t *testing.T) {
	h := NewHarness(t)

	_, err := h.Run("msg", "pub", "--topic", "test.list-a", "--sender", "test", "msg-a")
	require.NoError(t, err)
	_, err = h.Run("msg", "pub", "--topic", "test.list-b", "--sender", "test", "msg-b")
	require.NoError(t, err)

	out, err := h.RunStdout("msg", "list")
	require.NoError(t, err, "msg list: %s", out)

	lines, err := parseJSONLines(strings.TrimSpace(out))
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

	out, err := h.RunStdout("msg", "sub", "--topic", "nonexistent.topic.xyz")
	require.NoError(t, err, "msg sub empty: %s", out)
	assert.Empty(t, strings.TrimSpace(out))
}

func TestMsgInbox(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "inbox-repo")

	_, err := h.Run("new", "--remote", repo, "inbox-test")
	require.NoError(t, err)

	out, err := h.RunStdout("ls", "--json")
	require.NoError(t, err)

	lines, err := parseJSONLines(strings.TrimSpace(out))
	require.NoError(t, err)
	require.Len(t, lines, 1)

	inboxTopic, ok := lines[0]["inbox"].(string)
	require.True(t, ok, "session missing 'inbox' string field: %v", lines[0])
	require.NotEmpty(t, inboxTopic)

	_, err = h.Run("msg", "pub", "--topic", inboxTopic, "--sender", "test", "inbox message")
	require.NoError(t, err)

	subOut, err := h.RunStdout("msg", "sub", "--topic", inboxTopic)
	require.NoError(t, err, "sub inbox: %s", subOut)

	subLines, err := parseJSONLines(strings.TrimSpace(subOut))
	require.NoError(t, err)
	require.Len(t, subLines, 1)
	assert.Equal(t, "inbox message", subLines[0]["payload"])
}
