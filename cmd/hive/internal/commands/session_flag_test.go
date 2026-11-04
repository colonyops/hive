package commands

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/internal/domain/hc"
	"github.com/colonyops/hive/internal/domain/messaging"
	"github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/internal/ops"
)

const flagTestRemote = "https://github.com/acme/site"

func TestNoCommandHasASessionFlag(t *testing.T) {
	root := commandTree(t)

	var walk func(path string, c *cli.Command)
	walk = func(path string, c *cli.Command) {
		for _, f := range c.Flags {
			for _, name := range f.Names() {
				assert.NotEqual(t, "session", name, "%s has a --session flag", path)
			}
		}
		for _, sub := range c.Commands {
			walk(path+" "+sub.Name, sub)
		}
	}
	walk(root.Name, root)
}

func TestParseHCSessionID(t *testing.T) {
	t.Parallel()

	for value, want := range map[string]string{"": "", "abc": "abc", "local:abc": "abc"} {
		got, err := parseHCSessionID(value)
		require.NoError(t, err, value)
		assert.Equal(t, want, got, value)
	}

	_, err := parseHCSessionID("other:abc")
	var opsErr *ops.Error
	require.ErrorAs(t, err, &opsErr)
	assert.Equal(t, ops.KindInvalid, opsErr.Kind)
	assert.Contains(t, opsErr.Msg, "this machine")
}

func TestHCListSessionID(t *testing.T) {
	a := testApp(t)
	ctx := t.Context()

	epic, err := a.HC().CreateItem(ctx, "acme/site", hc.CreateItemInput{Title: "epic", Type: hc.ItemTypeEpic})
	require.NoError(t, err)
	mine, err := a.HC().CreateItem(ctx, "acme/site", hc.CreateItemInput{Title: "mine", Type: hc.ItemTypeTask, ParentID: epic.ID})
	require.NoError(t, err)
	_, err = a.HC().CreateItem(ctx, "acme/site", hc.CreateItemInput{Title: "other", Type: hc.ItemTypeTask, ParentID: epic.ID})
	require.NoError(t, err)
	owner := "sess1"
	_, err = a.HC().UpdateItem(ctx, mine.ID, hc.ItemUpdate{SessionID: &owner})
	require.NoError(t, err)

	register := NewHoneycombCmd(&Flags{}, a).Register

	for _, value := range []string{"sess1", "local:sess1"} {
		out, err := runRoot(t, register, "hc", "list", "--all-repos", "--json", "--session-id", value)
		require.NoError(t, err, value)
		assert.Equal(t, 1, strings.Count(out, "\n"), "%s: %s", value, out)
		assert.Contains(t, out, mine.ID)
	}

	_, err = runRoot(t, register, "hc", "list", "--all-repos", "--session-id", "other:sess1")
	var opsErr *ops.Error
	require.ErrorAs(t, err, &opsErr)
	assert.Equal(t, ops.KindInvalid, opsErr.Kind)
}

func TestMsgInboxCaller(t *testing.T) {
	t.Chdir(t.TempDir())
	a := testApp(t)
	ctx := t.Context()

	for _, name := range []string{"alpha", "beta"} {
		sess, err := a.SessionAPI().CreateSession(ctx, session.CreateOptions{Name: name, Remote: flagTestRemote, SkipSpawn: true})
		require.NoError(t, err)
		_, err = a.Messages().Publish(ctx, messaging.Message{Payload: "for-" + name}, []string{"agent." + sess.ID + ".inbox"})
		require.NoError(t, err)
	}
	register := NewMsgCmd(&Flags{}, a).Register

	t.Run("flag", func(t *testing.T) {
		out, err := runRoot(t, register, "msg", "inbox", "--caller", "alpha")
		require.NoError(t, err)
		assert.Contains(t, out, "for-alpha")
		assert.NotContains(t, out, "for-beta")
	})

	t.Run("env", func(t *testing.T) {
		t.Setenv("HIVE_AGENT_SESSION", "beta")
		out, err := runRoot(t, register, "msg", "inbox")
		require.NoError(t, err)
		assert.Contains(t, out, "for-beta")
		assert.NotContains(t, out, "for-alpha")
	})

	t.Run("flag beats env", func(t *testing.T) {
		t.Setenv("HIVE_AGENT_SESSION", "beta")
		out, err := runRoot(t, register, "msg", "inbox", "--caller", "alpha")
		require.NoError(t, err)
		assert.Contains(t, out, "for-alpha")
	})

	t.Run("neither", func(t *testing.T) {
		t.Setenv("HIVE_AGENT_SESSION", "")
		_, err := runRoot(t, register, "msg", "inbox")
		require.ErrorContains(t, err, "--caller")
	})
}
