package stores

import (
	"context"
	"testing"
	"time"

	"github.com/hay-kot/hive/internal/core/notify"
	"github.com/hay-kot/hive/internal/data/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotifyStore(t *testing.T) {
	ctx := context.Background()

	t.Run("save and list", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		store := NewNotifyStore(database)

		now := time.Now()
		id, err := store.Save(ctx, notify.Notification{
			Level:     notify.LevelError,
			Message:   "something broke",
			CreatedAt: now,
		})
		require.NoError(t, err)
		assert.Positive(t, id)

		items, err := store.List(ctx)
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, notify.LevelError, items[0].Level)
		assert.Equal(t, "something broke", items[0].Message)
		assert.Equal(t, id, items[0].ID)
	})

	t.Run("list returns newest first", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		store := NewNotifyStore(database)

		base := time.Now()
		for i, msg := range []string{"first", "second", "third"} {
			_, err := store.Save(ctx, notify.Notification{
				Level:     notify.LevelInfo,
				Message:   msg,
				CreatedAt: base.Add(time.Duration(i) * time.Second),
			})
			require.NoError(t, err)
		}

		items, err := store.List(ctx)
		require.NoError(t, err)
		require.Len(t, items, 3)
		assert.Equal(t, "third", items[0].Message)
		assert.Equal(t, "second", items[1].Message)
		assert.Equal(t, "first", items[2].Message)
	})

	t.Run("clear deletes all", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		store := NewNotifyStore(database)

		_, err = store.Save(ctx, notify.Notification{
			Level:     notify.LevelWarning,
			Message:   "warn",
			CreatedAt: time.Now(),
		})
		require.NoError(t, err)

		require.NoError(t, store.Clear(ctx))

		items, err := store.List(ctx)
		require.NoError(t, err)
		assert.Empty(t, items)
	})

	t.Run("count", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		store := NewNotifyStore(database)

		count, err := store.Count(ctx)
		require.NoError(t, err)
		assert.Equal(t, int64(0), count)

		for i := range 3 {
			_, err := store.Save(ctx, notify.Notification{
				Level:     notify.LevelInfo,
				Message:   "msg",
				CreatedAt: time.Now().Add(time.Duration(i) * time.Millisecond),
			})
			require.NoError(t, err)
		}

		count, err = store.Count(ctx)
		require.NoError(t, err)
		assert.Equal(t, int64(3), count)
	})

	t.Run("empty list returns empty slice", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err)
		defer func() { _ = database.Close() }()

		store := NewNotifyStore(database)

		items, err := store.List(ctx)
		require.NoError(t, err)
		assert.Empty(t, items)
		assert.NotNil(t, items)
	})
}
