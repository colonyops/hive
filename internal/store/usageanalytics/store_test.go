package usageanalytics

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	domain "github.com/colonyops/hive/internal/domain/usageanalytics"
	"github.com/colonyops/hive/internal/store/usageanalytics/queries"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func captured(event domain.Event, surface string) domain.Captured {
	return domain.Captured{Event: event, ID: uuid.NewString(), RunID: uuid.NewString(), Surface: surface, AppVersion: "dev", ReleaseChannel: "development"}
}

func TestTwoWritersDedupClearAndReopen(t *testing.T) {
	dir := t.TempDir()
	a, err := Open(t.Context(), dir)
	require.NoError(t, err)
	defer func() { require.NoError(t, a.Close()) }()
	b, err := Open(t.Context(), dir)
	require.NoError(t, err)
	command, _ := domain.LookupCommand("ls")
	batch := []domain.Captured{captured(domain.CommandCompleted(command, true, time.Second), "cli"), captured(domain.SessionCreated("full", false), "desktop"), captured(domain.TerminalStarted(true), "desktop")}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, writer := range []*Store{a, b} {
		wg.Go(func() { errs <- writer.WriteBatch(t.Context(), batch) })
	}
	wg.Wait()
	require.NoError(t, <-errs)
	require.NoError(t, <-errs)
	summary, err := a.Summary(t.Context(), time.Now())
	require.NoError(t, err)
	require.Equal(t, Summary{1, 1, 1}, summary)
	before, err := queries.New(a.db).Metadata(t.Context())
	require.NoError(t, err)
	queued := captured(domain.SessionCreated("full", true), "cli")
	_, err = a.Clear(t.Context())
	require.NoError(t, err)
	require.NoError(t, b.WriteBatch(t.Context(), append(batch, queued)))
	summary, err = a.Summary(t.Context(), time.Now())
	require.NoError(t, err)
	require.Equal(t, Summary{}, summary)
	after, err := queries.New(b.db).Metadata(t.Context())
	require.NoError(t, err)
	require.NotEqual(t, before.InstallationID, after.InstallationID)
	fresh := captured(domain.SessionCreated("worktree", false), "cli")
	require.NoError(t, b.WriteBatch(t.Context(), []domain.Captured{fresh}))
	var installation string
	require.NoError(t, a.db.QueryRowContext(t.Context(), "SELECT installation_id FROM usage_event").Scan(&installation))
	require.Equal(t, after.InstallationID, installation)
	require.NoError(t, b.Close())
	history, err := ReadSummary(t.Context(), dir, time.Now())
	require.NoError(t, err)
	require.Equal(t, Summary{HiveSessions: 1}, history)
}

func TestConcurrentFirstOpen(t *testing.T) {
	dir := t.TempDir()
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Go(func() {
			history, err := Open(t.Context(), dir)
			if err == nil {
				err = history.Close()
			}
			errs <- err
		})
	}
	wg.Wait()
	for range 8 {
		require.NoError(t, <-errs)
	}
}

func TestRetentionAndMissingRead(t *testing.T) {
	dir := t.TempDir()
	history, err := ReadSummary(t.Context(), dir, time.Now())
	require.NoError(t, err)
	require.Equal(t, Summary{}, history)
	_, err = os.Stat(filepath.Join(dir, Filename))
	require.True(t, os.IsNotExist(err))
	store, err := Open(t.Context(), dir)
	require.NoError(t, err)
	defer func() { require.NoError(t, store.Close()) }()
	batch := make([]domain.Captured, 1001)
	for i := range batch {
		batch[i] = captured(domain.TerminalStarted(false), "desktop")
	}
	require.NoError(t, store.WriteBatch(t.Context(), batch))
	now := time.Now().UTC()
	_, err = store.db.ExecContext(t.Context(), "UPDATE usage_event SET occurred_at_ms = ?", now.AddDate(0, 0, -91).UnixMilli())
	require.NoError(t, err)
	summary, err := store.Summary(t.Context(), now)
	require.NoError(t, err)
	require.Equal(t, Summary{}, summary)
	require.NoError(t, store.Prune(t.Context(), now))
	var count int
	require.NoError(t, store.db.QueryRowContext(t.Context(), "SELECT count(*) FROM usage_event").Scan(&count))
	require.Zero(t, count)
}

func BenchmarkBatches(b *testing.B) {
	for _, size := range []int{1, 8, 32, 64} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			store, err := Open(b.Context(), b.TempDir())
			if err != nil {
				b.Fatal(err)
			}
			defer func() {
				if err := store.Close(); err != nil {
					b.Error(err)
				}
			}()
			b.ResetTimer()
			for b.Loop() {
				batch := make([]domain.Captured, size)
				for i := range batch {
					batch[i] = captured(domain.TerminalStarted(false), "desktop")
				}
				if err := store.WriteBatch(b.Context(), batch); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
