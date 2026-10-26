package store

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/store/db"
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

	// Note: recycled session discovery is handled by session.Service.findValidRecyclable in internal/hive/session,
	// which calls List and filters in Go so it can validate paths and handle multiple
	// corrupted candidates. There is no FindRecyclable on the store.

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

// TestSessionStoreUpdate_SerializesWritersOnSeparateConnections stands in
// for two hive processes: each opens its own pool on the same hive.db.
func TestSessionStoreUpdate_SerializesWritersOnSeparateConnections(t *testing.T) {
	dir := t.TempDir()
	open := func() *SessionStore {
		database, err := db.Open(dir, db.DefaultOpenOptions())
		require.NoError(t, err)
		t.Cleanup(func() { _ = database.Close() })
		return NewSessionStore(database)
	}
	first, second := open(), open()
	now := time.Now()
	require.NoError(t, first.Save(t.Context(), session.Session{
		ID: "s", Name: "s", Path: "/tmp/s", State: session.StateActive, CreatedAt: now, UpdatedAt: now,
	}))

	const writers = 10
	var wg sync.WaitGroup
	for i := range writers {
		store := first
		if i%2 == 1 {
			store = second
		}
		wg.Go(func() {
			_, err := store.Update(t.Context(), "s", func(sess *session.Session) error {
				return sess.EditTags([]string{fmt.Sprintf("tag-%d", i)}, nil)
			})
			assert.NoError(t, err)
		})
	}
	wg.Wait()

	got, err := first.Get(t.Context(), "s")
	require.NoError(t, err)
	assert.Len(t, got.Tags, writers)
}

func TestSessionStoreUpdate_LeavesTheSessionAloneWhenTheEditFails(t *testing.T) {
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	store := NewSessionStore(database)

	_, err = store.Update(t.Context(), "missing", func(*session.Session) error { return nil })
	require.ErrorIs(t, err, session.ErrNotFound)

	now := time.Now()
	require.NoError(t, store.Save(t.Context(), session.Session{
		ID: "s", Name: "s", Path: "/tmp/s", State: session.StateActive, Tags: []string{"auto"}, CreatedAt: now, UpdatedAt: now,
	}))
	sentinel := errors.New("edit failed")
	_, err = store.Update(t.Context(), "s", func(sess *session.Session) error {
		sess.Tags = nil
		return sentinel
	})
	require.ErrorIs(t, err, sentinel)
	got, err := store.Get(t.Context(), "s")
	require.NoError(t, err)
	assert.Equal(t, []string{"auto"}, got.Tags)
}
