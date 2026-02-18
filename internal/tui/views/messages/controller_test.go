package messages

import (
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/messaging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMsg(topic, sender, payload string) messaging.Message {
	return messaging.Message{
		Topic:     topic,
		Sender:    sender,
		Payload:   payload,
		CreatedAt: time.Now(),
	}
}

func TestController_Append(t *testing.T) {
	t.Run("accumulates messages", func(t *testing.T) {
		c := NewController()
		c.Append([]messaging.Message{newMsg("t1", "s1", "first")})
		c.Append([]messaging.Message{newMsg("t2", "s2", "second")})

		assert.Equal(t, 2, c.Len())
	})

	t.Run("displays newest first", func(t *testing.T) {
		c := NewController()
		c.Append([]messaging.Message{
			newMsg("t1", "s1", "oldest"),
			newMsg("t2", "s2", "newest"),
		})

		displayed := c.Displayed()
		require.Len(t, displayed, 2)
		assert.Equal(t, "newest", displayed[0].Payload)
		assert.Equal(t, "oldest", displayed[1].Payload)
	})

	t.Run("incremental append reverses correctly", func(t *testing.T) {
		c := NewController()
		c.Append([]messaging.Message{newMsg("t1", "s1", "first")})
		c.Append([]messaging.Message{newMsg("t2", "s2", "second")})

		displayed := c.Displayed()
		require.Len(t, displayed, 2)
		assert.Equal(t, "second", displayed[0].Payload)
		assert.Equal(t, "first", displayed[1].Payload)
	})

	t.Run("empty append is a no-op", func(t *testing.T) {
		c := NewController()
		c.Append([]messaging.Message{newMsg("t1", "s1", "first")})
		c.Append(nil)

		assert.Equal(t, 1, c.Len())
		assert.Len(t, c.Displayed(), 1)
	})
}

func TestController_Filter(t *testing.T) {
	setup := func() *Controller {
		c := NewController()
		c.Append([]messaging.Message{
			newMsg("agent.inbox", "alice", "hello world"),
			newMsg("system.log", "bob", "error occurred"),
			newMsg("agent.outbox", "alice", "goodbye"),
		})
		return c
	}

	t.Run("filters by topic", func(t *testing.T) {
		c := setup()
		c.StartFilter()
		c.AddFilterRune('a')
		c.AddFilterRune('g')
		c.AddFilterRune('e')
		c.AddFilterRune('n')
		c.AddFilterRune('t')
		c.ConfirmFilter()

		assert.Len(t, c.FilteredAt(), 2)
		assert.Equal(t, "agent", c.Filter())
	})

	t.Run("filters by sender", func(t *testing.T) {
		c := setup()
		c.StartFilter()
		c.AddFilterRune('b')
		c.AddFilterRune('o')
		c.AddFilterRune('b')
		c.ConfirmFilter()

		assert.Len(t, c.FilteredAt(), 1)
	})

	t.Run("filters by payload", func(t *testing.T) {
		c := setup()
		c.StartFilter()
		c.AddFilterRune('e')
		c.AddFilterRune('r')
		c.AddFilterRune('r')
		c.AddFilterRune('o')
		c.AddFilterRune('r')
		c.ConfirmFilter()

		assert.Len(t, c.FilteredAt(), 1)
	})

	t.Run("case insensitive", func(t *testing.T) {
		c := setup()
		c.StartFilter()
		c.AddFilterRune('A')
		c.AddFilterRune('L')
		c.AddFilterRune('I')
		c.AddFilterRune('C')
		c.AddFilterRune('E')
		c.ConfirmFilter()

		assert.Len(t, c.FilteredAt(), 2)
	})

	t.Run("cancel restores full list", func(t *testing.T) {
		c := setup()
		c.StartFilter()
		c.AddFilterRune('b')
		c.AddFilterRune('o')
		c.AddFilterRune('b')
		c.ConfirmFilter()
		assert.Len(t, c.FilteredAt(), 1)

		c.CancelFilter()
		assert.Len(t, c.FilteredAt(), 3)
		assert.Empty(t, c.Filter())
		assert.False(t, c.IsFiltering())
	})

	t.Run("delete rune narrows then widens", func(t *testing.T) {
		c := NewController()
		c.Append([]messaging.Message{
			newMsg("topic.one", "alice", "hello"),
			newMsg("topic.two", "alice", "help"),
			newMsg("topic.three", "alice", "world"),
		})

		c.StartFilter()
		c.AddFilterRune('h')
		c.AddFilterRune('e')
		c.AddFilterRune('l')
		c.AddFilterRune('p')
		assert.Len(t, c.FilteredAt(), 1) // only "help"

		c.DeleteFilterRune()             // filter is now "hel"
		assert.Len(t, c.FilteredAt(), 2) // "hello" and "help"
		assert.Equal(t, "hel", c.Filter())
	})
}

func TestController_Navigation(t *testing.T) {
	setup := func() *Controller {
		c := NewController()
		c.Append([]messaging.Message{
			newMsg("t1", "s1", "msg1"),
			newMsg("t2", "s2", "msg2"),
			newMsg("t3", "s3", "msg3"),
		})
		return c
	}

	t.Run("move down increments cursor", func(t *testing.T) {
		c := setup()
		assert.Equal(t, 0, c.Cursor())

		c.MoveDown(10)
		assert.Equal(t, 1, c.Cursor())

		c.MoveDown(10)
		assert.Equal(t, 2, c.Cursor())
	})

	t.Run("move down clamps at bottom", func(t *testing.T) {
		c := setup()
		c.MoveDown(10)
		c.MoveDown(10)
		c.MoveDown(10) // should not go past 2
		c.MoveDown(10)

		assert.Equal(t, 2, c.Cursor())
	})

	t.Run("move up decrements cursor", func(t *testing.T) {
		c := setup()
		c.MoveDown(10)
		c.MoveDown(10)
		assert.Equal(t, 2, c.Cursor())

		c.MoveUp(10)
		assert.Equal(t, 1, c.Cursor())
	})

	t.Run("move up clamps at top", func(t *testing.T) {
		c := setup()
		c.MoveUp(10)
		assert.Equal(t, 0, c.Cursor())
	})
}

func TestController_Selected(t *testing.T) {
	t.Run("returns message at cursor", func(t *testing.T) {
		c := NewController()
		c.Append([]messaging.Message{
			newMsg("t1", "s1", "first"),
			newMsg("t2", "s2", "second"),
			newMsg("t3", "s3", "third"),
		})

		// Cursor at 0, displayed is reversed so newest first
		sel := c.Selected()
		require.NotNil(t, sel)
		assert.Equal(t, "third", sel.Payload)

		c.MoveDown(10)
		sel = c.Selected()
		require.NotNil(t, sel)
		assert.Equal(t, "second", sel.Payload)
	})

	t.Run("returns nil when empty", func(t *testing.T) {
		c := NewController()
		assert.Nil(t, c.Selected())
	})
}
