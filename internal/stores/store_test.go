package stores

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hay-kot/hive/internal/core/session"
	"github.com/hay-kot/hive/internal/data/db"
)

func TestStore(t *testing.T) {
	ctx := context.Background()

	t.Run("save and get", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
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

		if err := store.Save(ctx, sess); err != nil {
			t.Fatalf("Save: %v", err)
		}

		got, err := store.Get(ctx, "test-id")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}

		if got.ID != sess.ID || got.Name != sess.Name {
			t.Errorf("got %+v, want %+v", got, sess)
		}
	})

	t.Run("get not found", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer func() { _ = database.Close() }()

		store := NewSessionStore(database)

		_, err = store.Get(ctx, "nonexistent")
		if !errors.Is(err, session.ErrNotFound) {
			t.Errorf("got %v, want ErrNotFound", err)
		}
	})

	t.Run("list", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer func() { _ = database.Close() }()

		store := NewSessionStore(database)

		sessions, err := store.List(ctx)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(sessions) != 0 {
			t.Errorf("got %d sessions, want 0", len(sessions))
		}

		for _, name := range []string{"first", "second"} {
			if err := store.Save(ctx, session.Session{
				ID:    name,
				Name:  name,
				State: session.StateActive,
			}); err != nil {
				t.Fatalf("Save %s: %v", name, err)
			}
		}

		sessions, err = store.List(ctx)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(sessions) != 2 {
			t.Errorf("got %d sessions, want 2", len(sessions))
		}
	})

	t.Run("save updates existing", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer func() { _ = database.Close() }()

		store := NewSessionStore(database)

		sess := session.Session{
			ID:    "update-test",
			Name:  "original",
			State: session.StateActive,
		}
		if err := store.Save(ctx, sess); err != nil {
			t.Fatalf("Save: %v", err)
		}

		sess.Name = "updated"
		if err := store.Save(ctx, sess); err != nil {
			t.Fatalf("Save update: %v", err)
		}

		got, err := store.Get(ctx, "update-test")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.Name != "updated" {
			t.Errorf("got name %q, want %q", got.Name, "updated")
		}

		sessions, _ := store.List(ctx)
		if len(sessions) != 1 {
			t.Errorf("got %d sessions, want 1", len(sessions))
		}
	})

	t.Run("delete", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer func() { _ = database.Close() }()

		store := NewSessionStore(database)

		if err := store.Save(ctx, session.Session{
			ID:    "delete-me",
			State: session.StateActive,
		}); err != nil {
			t.Fatalf("Save: %v", err)
		}

		if err := store.Delete(ctx, "delete-me"); err != nil {
			t.Fatalf("Delete: %v", err)
		}

		_, err = store.Get(ctx, "delete-me")
		if !errors.Is(err, session.ErrNotFound) {
			t.Errorf("got %v, want ErrNotFound", err)
		}
	})

	t.Run("delete not found", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer func() { _ = database.Close() }()

		store := NewSessionStore(database)

		err = store.Delete(ctx, "nonexistent")
		if !errors.Is(err, session.ErrNotFound) {
			t.Errorf("got %v, want ErrNotFound", err)
		}
	})

	t.Run("find recyclable", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer func() { _ = database.Close() }()

		store := NewSessionStore(database)
		remote := "https://github.com/test/repo"

		// No recyclable sessions
		_, err = store.FindRecyclable(ctx, remote)
		if !errors.Is(err, session.ErrNoRecyclable) {
			t.Errorf("empty store: got %v, want ErrNoRecyclable", err)
		}

		// Active session with matching remote - not recyclable
		if err := store.Save(ctx, session.Session{
			ID:     "active",
			Remote: remote,
			State:  session.StateActive,
		}); err != nil {
			t.Fatalf("Save: %v", err)
		}

		_, err = store.FindRecyclable(ctx, remote)
		if !errors.Is(err, session.ErrNoRecyclable) {
			t.Errorf("active session: got %v, want ErrNoRecyclable", err)
		}

		// Recycled session with different remote - not found
		if err := store.Save(ctx, session.Session{
			ID:     "different",
			Remote: "https://github.com/other/repo",
			State:  session.StateRecycled,
		}); err != nil {
			t.Fatalf("Save: %v", err)
		}

		_, err = store.FindRecyclable(ctx, remote)
		if !errors.Is(err, session.ErrNoRecyclable) {
			t.Errorf("different remote: got %v, want ErrNoRecyclable", err)
		}

		// Recycled session with matching remote - found
		if err := store.Save(ctx, session.Session{
			ID:     "recycled",
			Remote: remote,
			State:  session.StateRecycled,
		}); err != nil {
			t.Fatalf("Save: %v", err)
		}

		got, err := store.FindRecyclable(ctx, remote)
		if err != nil {
			t.Fatalf("FindRecyclable: %v", err)
		}
		if got.ID != "recycled" {
			t.Errorf("got ID %q, want %q", got.ID, "recycled")
		}
	})

	t.Run("metadata serialization", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
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

		if err := store.Save(ctx, sess); err != nil {
			t.Fatalf("Save: %v", err)
		}

		got, err := store.Get(ctx, "metadata-test")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}

		if len(got.Metadata) != 2 {
			t.Errorf("got %d metadata entries, want 2", len(got.Metadata))
		}
		if got.Metadata["key1"] != "value1" {
			t.Errorf("got metadata[key1] %q, want %q", got.Metadata["key1"], "value1")
		}
	})

	t.Run("nil vs empty metadata", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer func() { _ = database.Close() }()

		store := NewSessionStore(database)

		// Test nil metadata
		sessNil := session.Session{
			ID:       "nil-metadata",
			State:    session.StateActive,
			Metadata: nil,
		}

		if err := store.Save(ctx, sessNil); err != nil {
			t.Fatalf("Save nil metadata: %v", err)
		}

		gotNil, err := store.Get(ctx, "nil-metadata")
		if err != nil {
			t.Fatalf("Get nil metadata: %v", err)
		}

		// Both nil and empty metadata should be stored as NULL in DB
		// and returned as empty map (not nil)
		if gotNil.Metadata == nil {
			gotNil.Metadata = make(map[string]string)
		}
		if len(gotNil.Metadata) != 0 {
			t.Errorf("nil metadata should have 0 entries, got %d", len(gotNil.Metadata))
		}

		// Test empty metadata
		sessEmpty := session.Session{
			ID:       "empty-metadata",
			State:    session.StateActive,
			Metadata: map[string]string{},
		}

		if err := store.Save(ctx, sessEmpty); err != nil {
			t.Fatalf("Save empty metadata: %v", err)
		}

		gotEmpty, err := store.Get(ctx, "empty-metadata")
		if err != nil {
			t.Fatalf("Get empty metadata: %v", err)
		}

		// Empty metadata also stored as NULL
		if gotEmpty.Metadata == nil {
			gotEmpty.Metadata = make(map[string]string)
		}
		if len(gotEmpty.Metadata) != 0 {
			t.Errorf("empty metadata should have 0 entries, got %d", len(gotEmpty.Metadata))
		}

		// Verify both nil and empty behave the same way
		if (gotNil.Metadata == nil) != (gotEmpty.Metadata == nil) {
			t.Error("nil and empty metadata should both round-trip the same way")
		}
	})
}
