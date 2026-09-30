package ingest

import (
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
)

func TestProducer_SetInterval(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		db := openTestPipelineDB(t)
		var mu sync.Mutex
		wakes := 0
		producer := newTestProducer(db, sourcesOf(map[string]connector.PullSource{"flow/s1": &fakeSource{}}), time.Hour, func(int64) {
			mu.Lock()
			wakes++
			mu.Unlock()
		}, zerolog.Nop())
		producer.Start(t.Context())
		producer.SetInterval(10 * time.Millisecond)
		time.Sleep(10 * time.Millisecond)
		synctest.Wait()
		producer.Stop()

		mu.Lock()
		defer mu.Unlock()
		require.Equal(t, 1, wakes, "the reset ticker should tick at the new cadence")
	})
}
