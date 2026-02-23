package todo

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTodoConstructors(t *testing.T) {
	t.Run("new agent todo requires session ID", func(t *testing.T) {
		_, err := NewAgentTodo("t1", "test", "", MustParseRef("review://doc.md"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "session ID is required")
	})

	t.Run("new agent todo sets source and session", func(t *testing.T) {
		td, err := NewAgentTodo("t1", "test", "sess-1", MustParseRef("review://doc.md"))
		require.NoError(t, err)
		assert.Equal(t, SourceAgent, td.Source)
		assert.Equal(t, "sess-1", td.SessionID)
	})

	t.Run("new human todo has no session", func(t *testing.T) {
		td, err := NewHumanTodo("t1", "test", MustParseRef("review://doc.md"))
		require.NoError(t, err)
		assert.Equal(t, SourceHuman, td.Source)
		assert.Empty(t, td.SessionID)
	})

	t.Run("new system todo has no session", func(t *testing.T) {
		td, err := NewSystemTodo("t1", "test", MustParseRef("review://doc.md"))
		require.NoError(t, err)
		assert.Equal(t, SourceSystem, td.Source)
		assert.Empty(t, td.SessionID)
	})

	t.Run("validate rejects non-agent todo with session ID", func(t *testing.T) {
		td := Todo{
			ID:        "t1",
			Source:    SourceHuman,
			SessionID: "sess-1",
			Title:     "test",
			Status:    StatusPending,
		}

		err := td.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "session ID is only valid")
	})
}
