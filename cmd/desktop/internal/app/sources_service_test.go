package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A mock mode builds no producer, so both entry points must report that
// rather than panicking on the nil the mode leaves behind.
func TestSourcesServiceWithoutAProducerIsUnavailable(t *testing.T) {
	t.Parallel()
	svc := newSourcesService(nil, nil, nil)

	_, err := svc.Refresh(t.Context())
	require.Error(t, err)
	assert.Equal(t, KindUnavailable, KindOf(err))

	_, err = svc.Run(t.Context())
	require.Error(t, err)
	assert.Equal(t, KindUnavailable, KindOf(err))
}
