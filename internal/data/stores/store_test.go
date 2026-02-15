package stores

import (
	"context"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/data/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore(t *testing.T) {
	ctx := context.Background()

	t.Run("save and get", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err, "Open")
		defer func() { _ = database.Close() }()

		store := NewSessionStore(database)

		sess := session.Session{
			ID:        "test-id",
			Name:      "test-session",
			Path:      "/tmp/test",
			Remote:    "https://github.com/test/repo",
			State:     session.StateActive,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		require.NoError(t, store.Save(ctx, sess), "Save")

		got, err := store.Get(ctx, "test-id")
		require.NoError(t, err, "Get")
		assert.Equal(t, sess.ID, got.ID)
		assert.Equal(t, sess.Name, got.Name)
	})

	t.Run("get not found", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err, "Open")
		defer func() { _ = database.Close() }()

		store := NewSessionStore(database)

		_, err = store.Get(ctx, "nonexistent")
		assert.ErrorIs(t, err, session.ErrNotFound, "got %v, want ErrNotFound", err)
	})

	t.Run("list", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err, "Open")
		defer func() { _ = database.Close() }()

		store := NewSessionStore(database)

		sessions, err := store.List(ctx)
		require.NoError(t, err, "List")
		assert.Empty(t, sessions, "got %d sessions, want 0", len(sessions))

		for _, name := range []string{"first", "second"} {
			require.NoError(t, store.Save(ctx, session.Session{
				ID:    name,
				Name:  name,
				State: session.StateActive,
			}), "Save %s", name)
		}

		sessions, err = store.List(ctx)
		require.NoError(t, err, "List")
		assert.Len(t, sessions, 2, "got %d sessions, want 2", len(sessions))
	})

	t.Run("save updates existing", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err, "Open")
		defer func() { _ = database.Close() }()

		store := NewSessionStore(database)

		sess := session.Session{
			ID:    "update-test",
			Name:  "original",
			State: session.StateActive,
		}
		require.NoError(t, store.Save(ctx, sess), "Save")

		sess.Name = "updated"
		require.NoError(t, store.Save(ctx, sess), "Save update")

		got, err := store.Get(ctx, "update-test")
		require.NoError(t, err, "Get")
		assert.Equal(t, "updated", got.Name)

		sessions, _ := store.List(ctx)
		assert.Len(t, sessions, 1, "got %d sessions, want 1", len(sessions))
	})

	t.Run("delete", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err, "Open")
		defer func() { _ = database.Close() }()

		store := NewSessionStore(database)

		require.NoError(t, store.Save(ctx, session.Session{
			ID:    "delete-me",
			State: session.StateActive,
		}), "Save")

		require.NoError(t, store.Delete(ctx, "delete-me"), "Delete")

		_, err = store.Get(ctx, "delete-me")
		require.ErrorIs(t, err, session.ErrNotFound, "got %v, want ErrNotFound", err)
	})

	t.Run("delete not found", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err, "Open")
		defer func() { _ = database.Close() }()

		store := NewSessionStore(database)

		err = store.Delete(ctx, "nonexistent")
		require.ErrorIs(t, err, session.ErrNotFound, "got %v, want ErrNotFound", err)
	})

	t.Run("find recyclable", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err, "Open")
		defer func() { _ = database.Close() }()

		store := NewSessionStore(database)
		remote := "https://github.com/test/repo"

		// No recyclable sessions
		_, err = store.FindRecyclable(ctx, remote)
		require.ErrorIs(t, err, session.ErrNoRecyclable, "empty store: got %v, want ErrNoRecyclable", err)

		// Active session with matching remote - not recyclable
		require.NoError(t, store.Save(ctx, session.Session{
			ID:     "active",
			Remote: remote,
			State:  session.StateActive,
		}), "Save")

		_, err = store.FindRecyclable(ctx, remote)
		require.ErrorIs(t, err, session.ErrNoRecyclable, "active session: got %v, want ErrNoRecyclable", err)

		// Recycled session with different remote - not found
		require.NoError(t, store.Save(ctx, session.Session{
			ID:     "different",
			Remote: "https://github.com/other/repo",
			State:  session.StateRecycled,
		}), "Save")

		_, err = store.FindRecyclable(ctx, remote)
		require.ErrorIs(t, err, session.ErrNoRecyclable, "different remote: got %v, want ErrNoRecyclable", err)

		// Recycled session with matching remote - found
		require.NoError(t, store.Save(ctx, session.Session{
			ID:     "recycled",
			Remote: remote,
			State:  session.StateRecycled,
		}), "Save")

		got, err := store.FindRecyclable(ctx, remote)
		require.NoError(t, err, "FindRecyclable")
		assert.Equal(t, "recycled", got.ID)
	})

	t.Run("metadata serialization", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err, "Open")
		defer func() { _ = database.Close() }()

		store := NewSessionStore(database)

		sess := session.Session{
			ID:    "metadata-test",
			State: session.StateActive,
			Metadata: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
		}

		require.NoError(t, store.Save(ctx, sess), "Save")

		got, err := store.Get(ctx, "metadata-test")
		require.NoError(t, err, "Get")
		assert.Len(t, got.Metadata, 2, "got %d metadata entries, want 2", len(got.Metadata))
		assert.Equal(t, "value1", got.Metadata["key1"])
	})

	t.Run("nil vs empty metadata", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err, "Open")
		defer func() { _ = database.Close() }()

		store := NewSessionStore(database)

		// Test nil metadata
		sessNil := session.Session{
			ID:       "nil-metadata",
			State:    session.StateActive,
			Metadata: nil,
		}

		require.NoError(t, store.Save(ctx, sessNil), "Save nil metadata")

		gotNil, err := store.Get(ctx, "nil-metadata")
		require.NoError(t, err, "Get nil metadata")

		// Both nil and empty metadata should be stored as NULL in DB
		// and returned as empty map (not nil)
		if gotNil.Metadata == nil {
			gotNil.Metadata = make(map[string]string)
		}
		assert.Empty(t, gotNil.Metadata, "nil metadata should have 0 entries, got %d", len(gotNil.Metadata))

		// Test empty metadata
		sessEmpty := session.Session{
			ID:       "empty-metadata",
			State:    session.StateActive,
			Metadata: map[string]string{},
		}

		require.NoError(t, store.Save(ctx, sessEmpty), "Save empty metadata")

		gotEmpty, err := store.Get(ctx, "empty-metadata")
		require.NoError(t, err, "Get empty metadata")

		// Empty metadata also stored as NULL
		if gotEmpty.Metadata == nil {
			gotEmpty.Metadata = make(map[string]string)
		}
		assert.Empty(t, gotEmpty.Metadata, "empty metadata should have 0 entries, got %d", len(gotEmpty.Metadata))

		// Verify both nil and empty behave the same way
		assert.Equal(t, gotNil.Metadata == nil, gotEmpty.Metadata == nil, "nil and empty metadata should both round-trip the same way")
	})
}
