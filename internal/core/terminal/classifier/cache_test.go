package classifier_test

import (
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/terminal/classifier"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCache_HitOnSamePID(t *testing.T) {
	cache := classifier.NewCache()
	want := classifier.Result{IsAgent: true, Tool: testToolClaude, Confidence: classifier.ConfidenceHigh, Tier: 2, ClassifiedAt: time.Now()}
	cache.Set("%1", 100, want)

	got, ok := cache.Get("%1", 100)
	require.True(t, ok)
	assert.Equal(t, want, got)
	assert.Equal(t, 1, cache.Len())
}

func TestCache_MissOnPIDChange(t *testing.T) {
	cache := classifier.NewCache()
	cache.Set("%1", 100, classifier.Result{IsAgent: true, Tool: testToolClaude})

	got, ok := cache.Get("%1", 101)
	assert.False(t, ok)
	assert.Equal(t, classifier.Result{}, got)
}

func TestCache_Prune(t *testing.T) {
	cache := classifier.NewCache()
	cache.Set("%1", 100, classifier.Result{Tool: testToolClaude})
	cache.Set("%2", 200, classifier.Result{Tool: testToolCodex})
	cache.Set("%3", 300, classifier.Result{Tool: testToolGemini})

	cache.Prune(map[string]bool{"%1": true, "%3": true})

	assert.Equal(t, 2, cache.Len())
	_, ok := cache.Get("%1", 100)
	assert.True(t, ok)
	_, ok = cache.Get("%2", 200)
	assert.False(t, ok)
	_, ok = cache.Get("%3", 300)
	assert.True(t, ok)
}

func TestCache_PIDReuse(t *testing.T) {
	cache := classifier.NewCache()
	cache.Set("%1", 100, classifier.Result{IsAgent: true, Tool: testToolClaude})
	_, ok := cache.Get("%1", 101)
	assert.False(t, ok, "PID change must force reclassification before a reused pane can be trusted")
}

func TestCache_NilSafe(t *testing.T) {
	var cache *classifier.Cache
	assert.Equal(t, 0, cache.Len())
	_, ok := cache.Get("%1", 100)
	assert.False(t, ok)
	assert.NotPanics(t, func() { cache.Prune(map[string]bool{"%1": true}) })
}
