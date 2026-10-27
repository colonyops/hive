package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/colonyops/hive/internal/store/db"
)

func TestDriveUploadStore(t *testing.T) {
	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	s := NewDriveUploadStore(database)

	repo, err := drive.RepositorySpace("colonyops", "hive")
	require.NoError(t, err)
	at := time.Unix(1_700_000_000, 0)
	upload := func(server string, space drive.Space, path, sum string) drive.Upload {
		return drive.Upload{ID: server + path + sum, Server: server, Space: space, Path: path, SHA256: sum, UploadedAt: at}
	}

	t.Run("record replaces the row for the same server, space, and path", func(t *testing.T) {
		require.NoError(t, s.Record(t.Context(), upload("drive.test", repo, "a.md", "one")))
		later := upload("drive.test", repo, "a.md", "two")
		later.UploadedAt = at.Add(time.Hour)
		require.NoError(t, s.Record(t.Context(), later))

		got, err := s.List(t.Context(), "drive.test", repo)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "two", got["a.md"].SHA256)
		assert.Equal(t, repo, got["a.md"].Space)
		assert.True(t, later.UploadedAt.Equal(got["a.md"].UploadedAt))
	})

	t.Run("servers and spaces do not mix", func(t *testing.T) {
		require.NoError(t, s.Record(t.Context(), upload("other.test", repo, "a.md", "three")))
		require.NoError(t, s.Record(t.Context(), upload("drive.test", drive.Shared, "a.md", "four")))

		got, err := s.List(t.Context(), "drive.test", repo)
		require.NoError(t, err)
		assert.Equal(t, "two", got["a.md"].SHA256)

		other, err := s.List(t.Context(), "other.test", repo)
		require.NoError(t, err)
		assert.Equal(t, "three", other["a.md"].SHA256)

		shared, err := s.List(t.Context(), "drive.test", drive.Shared)
		require.NoError(t, err)
		assert.Equal(t, "four", shared["a.md"].SHA256)
	})

	t.Run("an unknown space lists empty", func(t *testing.T) {
		got, err := s.List(t.Context(), "nowhere.test", repo)
		require.NoError(t, err)
		assert.Empty(t, got)
	})
}
