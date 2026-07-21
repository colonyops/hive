package pipeline

import (
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestProducer_SetInterval(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		db := openTestPipelineDB(t)
		var mu sync.Mutex
		wakes := 0
		producer := NewProducer(db, listerOf(map[string]Source{"s1": &fakeSource{}}), time.Hour, func(int64) {
			mu.Lock()
			wakes++
			mu.Unlock()
		}, zerolog.Nop())
		producer.Start()
		producer.SetInterval(10 * time.Millisecond)
		time.Sleep(10 * time.Millisecond)
		synctest.Wait()
		producer.Stop()

		mu.Lock()
		defer mu.Unlock()
		require.Equal(t, 1, wakes, "the reset ticker should tick at the new cadence")
	})
}
