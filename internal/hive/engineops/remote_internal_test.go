package engineops

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/hive/daemon/wire"
	"github.com/colonyops/hive/internal/ops"
)

func TestWaitOfReadsEveryNumberASurfaceSends(t *testing.T) {
	assert.Equal(t, 30*time.Second, waitOf(map[string]any{"wait": int64(30)}))
	assert.Equal(t, 30*time.Second, waitOf(map[string]any{"wait": 30}))
	assert.Equal(t, 1500*time.Millisecond, waitOf(map[string]any{"wait": 1.5}))
	assert.Zero(t, waitOf(map[string]any{}))
}

func TestAVersionMismatchIsUnavailable(t *testing.T) {
	for _, err := range []error{&wire.IncompatibleError{}, &wire.CatalogError{}, &wire.UnknownToolError{}} {
		opErr, ok := errors.AsType[*ops.Error](opError(err, "call"))
		require.True(t, ok)
		assert.Equal(t, ops.KindUnavailable, opErr.Kind, "%T", err)
	}
}
