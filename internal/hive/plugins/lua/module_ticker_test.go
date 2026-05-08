package lua

import (
	"runtime"
	"testing"
	"testing/synctest"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tickerHarness wraps luaHarness with a TickerModule reference so tests can
// reach the module for explicit shutdown ordering.
type tickerHarness struct {
	*luaHarness
	module *TickerModule
}

func newTickerHarness(t *testing.T, script string) *tickerHarness {
	t.Helper()
	module := &TickerModule{Logger: zerolog.Nop()}
	return &tickerHarness{
		luaHarness: newLuaHarness(t, script, module),
		module:     module,
	}
}

func TestTickerEveryFiresRepeatedly(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		h := newTickerHarness(t, `
return function(hive)
  hive.ticker.every("1s", function() hive.test_bump() end)
end
`)

		// 2.5s of virtual time fires the 1s ticker exactly twice.
		time.Sleep(2500 * time.Millisecond)
		synctest.Wait()

		assert.Equal(t, int64(2), h.capture.Counter())
	})
}

func TestTickerAfterFiresOnce(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		h := newTickerHarness(t, `
return function(hive)
  hive.ticker.after("1s", function() hive.test_bump() end)
end
`)

		time.Sleep(2500 * time.Millisecond)
		synctest.Wait()

		assert.Equal(t, int64(1), h.capture.Counter())
	})
}

func TestTickerCancelStopsFurtherFires(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		// Callback cancels itself on first fire — exactly one bump expected.
		h := newTickerHarness(t, `
return function(hive)
  local handle
  handle = hive.ticker.every("1s", function()
    hive.test_bump()
    handle:cancel()
  end)
end
`)

		// 3s covers several would-be ticks if cancel were missed.
		time.Sleep(3 * time.Second)
		synctest.Wait()

		assert.Equal(t, int64(1), h.capture.Counter())
	})
}

func TestTickerCallbackErrorsKeepFiring(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		h := newTickerHarness(t, `
return function(hive)
  hive.ticker.every("1s", function()
    hive.test_bump()
    error("boom")
  end)
end
`)

		time.Sleep(2500 * time.Millisecond)
		synctest.Wait()

		assert.Equal(t, int64(2), h.capture.Counter())
	})
}

func TestTickerCloseCancelsAllOutstandingTickers(t *testing.T) {
	// Not parallel: runtime.NumGoroutine is process-wide.
	synctest.Test(t, func(t *testing.T) {
		synctest.Wait()
		before := runtime.NumGoroutine()

		h := newTickerHarness(t, `
return function(hive)
  hive.ticker.every("1s", function() hive.test_bump() end)
  hive.ticker.every("1s", function() hive.test_bump() end)
  hive.ticker.after("1s", function() hive.test_bump() end)
end
`)

		time.Sleep(1200 * time.Millisecond)
		synctest.Wait()
		require.Positive(t, h.capture.Counter(), "tickers should have fired before close")

		h.Close()
		synctest.Wait()
		after := runtime.NumGoroutine()

		// NumGoroutine is noisy; allow a small margin but flag obvious leaks.
		assert.LessOrEqual(t, after, before+2,
			"goroutine count should not grow appreciably after Close (before=%d, after=%d)", before, after)
	})
}

func TestTickerEveryRejectsSubSecondDuration(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		// The entrypoint asserts via pcall that the call errored with the
		// expected message; the counter stays zero since no ticker registers.
		h := newTickerHarness(t, `
return function(hive)
  local ok, err = pcall(hive.ticker.every, "500ms", function() hive.test_bump() end)
  if ok then
    error("expected sub-second duration to be rejected")
  end
  if not string.find(tostring(err), "duration must be at least") then
    error("unexpected error: " .. tostring(err))
  end
end
`)

		time.Sleep(1200 * time.Millisecond)
		synctest.Wait()
		assert.Equal(t, int64(0), h.capture.Counter(), "no ticker should have been registered")
	})
}
