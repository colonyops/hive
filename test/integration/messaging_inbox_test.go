//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sessionDir returns the working directory for the first session found in the data dir.
func sessionDir(t *testing.T, h *Harness) string {
	t.Helper()
	reposDir := filepath.Join(h.DataDir(), "repos")
	entries, err := os.ReadDir(reposDir)
	require.NoError(t, err, "reading repos dir")
	require.NotEmpty(t, entries, "expected at least one session repo")
	return filepath.Join(reposDir, entries[0].Name())
}

func TestSubDefaultDoesNotAcknowledge(t *testing.T) {
	h := NewHarness(t)

	_, err := h.Run("msg", "pub", "--topic", "test.noack", "--sender", "test", "noack msg")
	require.NoError(t, err)

	// Default sub should return the message without acknowledging
	lines, err := h.RunJSONLines("msg", "sub", "--topic", "test.noack")
	require.NoError(t, err)
	require.Len(t, lines, 1)
	assert.Equal(t, "noack msg", lines[0]["payload"])

	// Subsequent sub should still return the same message (not acknowledged)
	lines, err = h.RunJSONLines("msg", "sub", "--topic", "test.noack")
	require.NoError(t, err)
	require.Len(t, lines, 1)
	assert.Equal(t, "noack msg", lines[0]["payload"])
}

func TestSubTail(t *testing.T) {
	h := NewHarness(t)

	for i, msg := range []string{"first", "second", "third", "fourth"} {
		_, err := h.Run("msg", "pub", "--topic", "test.tail", "--sender", "test", msg)
		require.NoError(t, err, "pub message %d", i)
	}

	lines, err := h.RunJSONLines("msg", "sub", "--topic", "test.tail", "--tail", "2")
	require.NoError(t, err)
	require.Len(t, lines, 2)
	assert.Equal(t, "third", lines[0]["payload"])
	assert.Equal(t, "fourth", lines[1]["payload"])
}

func TestSubDeprecatedLastFlag(t *testing.T) {
	h := NewHarness(t)

	for _, msg := range []string{"a", "b", "c"} {
		_, err := h.Run("msg", "pub", "--topic", "test.last", "--sender", "test", "-m", msg)
		require.NoError(t, err)
	}

	// --last should still work as hidden alias
	lines, err := h.RunJSONLines("msg", "sub", "--topic", "test.last", "--last", "1")
	require.NoError(t, err)
	require.Len(t, lines, 1)
	assert.Equal(t, "c", lines[0]["payload"])
}

func TestSubSenderField(t *testing.T) {
	h := NewHarness(t)

	_, err := h.Run("msg", "pub", "--topic", "test.sender", "--sender", "agent-foo", "hello")
	require.NoError(t, err)

	lines, err := h.RunJSONLines("msg", "sub", "--topic", "test.sender")
	require.NoError(t, err)
	require.Len(t, lines, 1)
	assert.Equal(t, "agent-foo", lines[0]["sender"])
}

func TestSubWildcard(t *testing.T) {
	h := NewHarness(t)

	_, err := h.Run("msg", "pub", "--topic", "wild.alpha", "--sender", "test", "msg-alpha")
	require.NoError(t, err)
	_, err = h.Run("msg", "pub", "--topic", "wild.beta", "--sender", "test", "msg-beta")
	require.NoError(t, err)
	_, err = h.Run("msg", "pub", "--topic", "other.gamma", "--sender", "test", "msg-gamma")
	require.NoError(t, err)

	// Wildcard "wild.*" should match alpha and beta but not gamma
	lines, err := h.RunJSONLines("msg", "sub", "--topic", "wild.*")
	require.NoError(t, err)
	require.Len(t, lines, 2)

	payloads := []string{lines[0]["payload"].(string), lines[1]["payload"].(string)}
	assert.Contains(t, payloads, "msg-alpha")
	assert.Contains(t, payloads, "msg-beta")
}

func TestSubAllTopics(t *testing.T) {
	h := NewHarness(t)

	_, err := h.Run("msg", "pub", "--topic", "all.one", "--sender", "test", "one")
	require.NoError(t, err)
	_, err = h.Run("msg", "pub", "--topic", "all.two", "--sender", "test", "two")
	require.NoError(t, err)

	// No --topic defaults to "*" (all topics)
	lines, err := h.RunJSONLines("msg", "sub")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(lines), 2)

	payloads := make([]string, len(lines))
	for i, l := range lines {
		payloads[i] = l["payload"].(string)
	}
	assert.Contains(t, payloads, "one")
	assert.Contains(t, payloads, "two")
}

func TestWildcardPublish(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "wpub-repo")

	// Create two sessions so we have two inbox topics
	_, err := h.Run("new", "--remote", repo, "wpub-one")
	require.NoError(t, err)
	_, err = h.Run("new", "--remote", repo, "wpub-two")
	require.NoError(t, err)

	sessions, err := h.RunJSONLines("ls", "--json")
	require.NoError(t, err)
	require.Len(t, sessions, 2)

	inbox1 := sessions[0]["inbox"].(string)
	inbox2 := sessions[1]["inbox"].(string)

	// Seed the inbox topics so they exist for wildcard expansion
	_, err = h.Run("msg", "pub", "--topic", inbox1, "--sender", "test", "seed1")
	require.NoError(t, err)
	_, err = h.Run("msg", "pub", "--topic", inbox2, "--sender", "test", "seed2")
	require.NoError(t, err)

	// Wildcard publish to all inbox topics
	_, err = h.Run("msg", "pub", "--topic", "agent.*.inbox", "--sender", "test", "broadcast")
	require.NoError(t, err)

	// Both inboxes should have the broadcast message
	lines1, err := h.RunJSONLines("msg", "sub", "--topic", inbox1)
	require.NoError(t, err)
	var found1 bool
	for _, l := range lines1 {
		if l["payload"] == "broadcast" {
			found1 = true
			break
		}
	}
	assert.True(t, found1, "inbox1 should contain broadcast message")

	lines2, err := h.RunJSONLines("msg", "sub", "--topic", inbox2)
	require.NoError(t, err)
	var found2 bool
	for _, l := range lines2 {
		if l["payload"] == "broadcast" {
			found2 = true
			break
		}
	}
	assert.True(t, found2, "inbox2 should contain broadcast message")
}

func TestInboxUnreadTracking(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "unread-repo")

	_, err := h.Run("new", "--remote", repo, "unread-test")
	require.NoError(t, err)

	sessions, err := h.RunJSONLines("ls", "--json")
	require.NoError(t, err)
	require.Len(t, sessions, 1)

	inboxTopic := sessions[0]["inbox"].(string)
	dir := sessionDir(t, h)

	// Publish a message to the inbox
	_, err = h.Run("msg", "pub", "--topic", inboxTopic, "--sender", "test", "unread msg")
	require.NoError(t, err)

	// Read inbox with --ack from session dir (acknowledges)
	lines, err := h.RunJSONLinesInDir(dir, "msg", "inbox", "--ack")
	require.NoError(t, err)
	require.Len(t, lines, 1)
	assert.Equal(t, "unread msg", lines[0]["payload"])

	// Second read should return empty (already acknowledged)
	lines, err = h.RunJSONLinesInDir(dir, "msg", "inbox")
	require.NoError(t, err)
	assert.Empty(t, lines)
}

func TestInboxAllShowsAcknowledged(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "inboxall-repo")

	_, err := h.Run("new", "--remote", repo, "inboxall-test")
	require.NoError(t, err)

	sessions, err := h.RunJSONLines("ls", "--json")
	require.NoError(t, err)
	require.Len(t, sessions, 1)

	inboxTopic := sessions[0]["inbox"].(string)
	dir := sessionDir(t, h)

	_, err = h.Run("msg", "pub", "--topic", inboxTopic, "--sender", "test", "acked msg")
	require.NoError(t, err)

	// Read and acknowledge
	lines, err := h.RunJSONLinesInDir(dir, "msg", "inbox", "--ack")
	require.NoError(t, err)
	require.Len(t, lines, 1)

	// --all should still return the acknowledged message
	lines, err = h.RunJSONLinesInDir(dir, "msg", "inbox", "--all")
	require.NoError(t, err)
	require.Len(t, lines, 1)
	assert.Equal(t, "acked msg", lines[0]["payload"])
}

func TestInboxDefaultDoesNotAcknowledge(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "inboxdflt-repo")

	_, err := h.Run("new", "--remote", repo, "inboxdflt-test")
	require.NoError(t, err)

	sessions, err := h.RunJSONLines("ls", "--json")
	require.NoError(t, err)
	require.Len(t, sessions, 1)

	inboxTopic := sessions[0]["inbox"].(string)
	dir := sessionDir(t, h)

	_, err = h.Run("msg", "pub", "--topic", inboxTopic, "--sender", "test", "dflt msg")
	require.NoError(t, err)

	// Default inbox (no --ack) should not acknowledge
	lines, err := h.RunJSONLinesInDir(dir, "msg", "inbox")
	require.NoError(t, err)
	require.Len(t, lines, 1)
	assert.Equal(t, "dflt msg", lines[0]["payload"])

	// Reading again should still show the message (not acknowledged)
	lines, err = h.RunJSONLinesInDir(dir, "msg", "inbox")
	require.NoError(t, err)
	require.Len(t, lines, 1)
	assert.Equal(t, "dflt msg", lines[0]["payload"])
}

func TestInboxWithSessionFlag(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "sessfl-repo")

	_, err := h.Run("new", "--remote", repo, "sessfl-test")
	require.NoError(t, err)

	sessions, err := h.RunJSONLines("ls", "--json")
	require.NoError(t, err)
	require.Len(t, sessions, 1)

	inboxTopic := sessions[0]["inbox"].(string)
	sessionID := sessions[0]["id"].(string)
	sessionName := sessions[0]["name"].(string)

	_, err = h.Run("msg", "pub", "--topic", inboxTopic, "--sender", "test", "session-flag msg")
	require.NoError(t, err)

	// Use --session with ID (no need to be in session dir)
	lines, err := h.RunJSONLines("msg", "inbox", "--session", sessionID)
	require.NoError(t, err)
	require.Len(t, lines, 1)
	assert.Equal(t, "session-flag msg", lines[0]["payload"])

	// Use --session with name
	lines, err = h.RunJSONLines("msg", "inbox", "--session", sessionName)
	require.NoError(t, err)
	require.Len(t, lines, 1)
	assert.Equal(t, "session-flag msg", lines[0]["payload"])
}

func TestInboxTail(t *testing.T) {
	h := NewHarness(t)
	repo := createBareRepo(t, "inboxtail-repo")

	_, err := h.Run("new", "--remote", repo, "inboxtail-test")
	require.NoError(t, err)

	sessions, err := h.RunJSONLines("ls", "--json")
	require.NoError(t, err)
	require.Len(t, sessions, 1)

	inboxTopic := sessions[0]["inbox"].(string)
	dir := sessionDir(t, h)

	for _, msg := range []string{"first", "second", "third"} {
		_, err = h.Run("msg", "pub", "--topic", inboxTopic, "--sender", "test", msg)
		require.NoError(t, err)
	}

	lines, err := h.RunJSONLinesInDir(dir, "msg", "inbox", "--tail", "2")
	require.NoError(t, err)
	require.Len(t, lines, 2)
	assert.Equal(t, "second", lines[0]["payload"])
	assert.Equal(t, "third", lines[1]["payload"])
}

func TestPubFromStdin(t *testing.T) {
	h := NewHarness(t)

	out, err := h.RunWithStdin("stdin payload", "msg", "pub", "--topic", "test.stdin", "--sender", "test")
	require.NoError(t, err, "pub from stdin: %s", out)

	lines, err := h.RunJSONLines("msg", "sub", "--topic", "test.stdin")
	require.NoError(t, err)
	require.Len(t, lines, 1)
	assert.Equal(t, "stdin payload", lines[0]["payload"])
}

func TestPubWithMessageFlag(t *testing.T) {
	h := NewHarness(t)

	_, err := h.Run("msg", "pub", "--topic", "test.mflag", "--sender", "test", "-m", "inline msg")
	require.NoError(t, err)

	lines, err := h.RunJSONLines("msg", "sub", "--topic", "test.mflag")
	require.NoError(t, err)
	require.Len(t, lines, 1)
	assert.Equal(t, "inline msg", lines[0]["payload"])
}

func TestMessageFieldCompleteness(t *testing.T) {
	h := NewHarness(t)

	_, err := h.Run("msg", "pub", "--topic", "test.fields", "--sender", "field-check", "field payload")
	require.NoError(t, err)

	lines, err := h.RunJSONLines("msg", "sub", "--topic", "test.fields")
	require.NoError(t, err)
	require.Len(t, lines, 1)

	msg := lines[0]
	assert.NotEmpty(t, msg["id"], "message should have an id")
	assert.NotEmpty(t, msg["created_at"], "message should have created_at")
	assert.Equal(t, "test.fields", msg["topic"])
	assert.Equal(t, "field payload", msg["payload"])
	assert.Equal(t, "field-check", msg["sender"])
}

func TestMsgTopicCustomPrefix(t *testing.T) {
	h := NewHarness(t)

	out, err := h.RunStdout("msg", "topic", "--prefix", "custom")
	require.NoError(t, err)
	topic := strings.TrimSpace(out)
	assert.True(t, len(topic) > 0, "topic should not be empty")
	assert.Contains(t, topic, "custom.", "topic should start with custom prefix")
}

func TestMsgTopicEmptyPrefix(t *testing.T) {
	h := NewHarness(t)

	out, err := h.RunStdout("msg", "topic", "--prefix", "")
	require.NoError(t, err)
	topic := strings.TrimSpace(out)
	assert.True(t, len(topic) > 0, "topic should not be empty")
	assert.NotContains(t, topic, ".", "topic with empty prefix should have no dot")
}

func TestPubToMultipleTopics(t *testing.T) {
	h := NewHarness(t)

	_, err := h.Run("msg", "pub", "--topic", "multi.a", "--topic", "multi.b", "--sender", "test", "multi msg")
	require.NoError(t, err)

	linesA, err := h.RunJSONLines("msg", "sub", "--topic", "multi.a")
	require.NoError(t, err)
	require.Len(t, linesA, 1)
	assert.Equal(t, "multi msg", linesA[0]["payload"])

	linesB, err := h.RunJSONLines("msg", "sub", "--topic", "multi.b")
	require.NoError(t, err)
	require.Len(t, linesB, 1)
	assert.Equal(t, "multi msg", linesB[0]["payload"])
}
