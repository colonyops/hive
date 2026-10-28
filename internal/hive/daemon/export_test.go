package daemon

import (
	"testing"
	"time"
)

func SetOwnershipCheckInterval(t testing.TB, d time.Duration) {
	original := ownershipCheckInterval
	ownershipCheckInterval = d
	t.Cleanup(func() { ownershipCheckInterval = original })
}
