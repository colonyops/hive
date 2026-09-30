package tmux

import (
	"context"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/multiplexer"
	"github.com/colonyops/hive/internal/core/terminal/classifier"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type canceledPaneSource struct{}

func (canceledPaneSource) ListPanes(ctx context.Context) ([]multiplexer.Pane, error) {
	return nil, ctx.Err()
}

func (canceledPaneSource) CapturePane(context.Context, multiplexer.Target, multiplexer.CaptureOptions) (string, error) {
	return "", nil
}

func TestRefreshCacheCancellationPreservesDiscoverableCache(t *testing.T) {
	integration := NewFromPreviewMatchers(nil, WithPaneSource(canceledPaneSource{}))
	integration.cache = map[string]*sessionCache{
		"work": {panes: []cachedPane{{
			input: classifier.PaneInput{
				Target:      multiplexer.Target{Session: "work", Window: "0", Pane: "0"},
				SessionName: "work",
				WindowIndex: "0",
				PaneID:      "%1",
			},
			result: classifier.Result{IsAgent: true},
		}}},
	}
	integration.cacheTime = time.Now().Add(-time.Minute)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	integration.RefreshCache(ctx)

	assert.Contains(t, integration.cache, "work")
	assert.Zero(t, integration.refreshFailures)
	info, err := integration.DiscoverSession(context.Background(), "work", nil)
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "%1", info.PaneID)
}
