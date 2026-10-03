package store

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/domain/hc"
	"github.com/colonyops/hive/internal/store/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// UpdateItem and AddBlocker read before they write. Under deferred locking a
// CLI and a desktop handle running them at once fail with SQLITE_BUSY without
// waiting on busy_timeout.
func TestHCStore_UpdateItemAndAddBlockerAcrossHandles(t *testing.T) {
	const (
		tasks      = 12
		goroutines = 4
		updates    = 25
	)
	ctx := t.Context()
	dir := t.TempDir()

	openStore := func() *HCStore {
		database, err := db.Open(dir, db.DefaultOpenOptions())
		require.NoError(t, err)
		t.Cleanup(func() { _ = database.Close() })
		return NewHCStore(database)
	}
	updater := openStore()
	blocker := openStore()

	now := time.Now()
	items := []hc.Item{makeEpic("epic", now)}
	for i := range tasks {
		items = append(items, makeItem(taskID(i), "epic", "epic", hc.StatusOpen, 1, now))
	}
	require.NoError(t, updater.CreateItems(ctx, items))

	var pairs [][2]string
	for i := range tasks {
		for j := i + 1; j < tasks; j++ {
			pairs = append(pairs, [2]string{taskID(i), taskID(j)})
		}
	}

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)
	record := func(err error) {
		if err != nil {
			mu.Lock()
			errs = append(errs, err)
			mu.Unlock()
		}
	}
	for g := range goroutines {
		wg.Go(func() {
			for i := range updates {
				title := fmt.Sprintf("title %d-%d", g, i)
				_, err := updater.UpdateItem(ctx, taskID((g+i)%tasks), hc.ItemUpdate{Title: &title})
				record(err)
			}
		})
		wg.Go(func() {
			for i := g; i < len(pairs); i += goroutines {
				record(blocker.AddBlocker(ctx, pairs[i][0], pairs[i][1]))
			}
		})
	}
	wg.Wait()

	require.Empty(t, errs)
	edges, err := blocker.db.Queries().ListAllHCBlockerEdges(ctx)
	require.NoError(t, err)
	assert.Len(t, edges, len(pairs))
}

func taskID(i int) string { return fmt.Sprintf("task-%02d", i) }
