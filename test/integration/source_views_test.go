//go:build integration

package integration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSourceViewsConfigLoadsInDeclarationOrder(t *testing.T) {
	h := NewHarness(t).WithConfig(`
version: "0.2.7"
git_path: git
agents:
  default: testbash
  testbash:
    command: bash
sources:
  search_limit: 17
  cache_ttl: 2m
  views:
    - name: my-review-queue
      base: prs
      query: "review-requested:@me state:open"
    - name: triage
      base: issues
      query: "label:triage no:assignee archived:false"
      scope: "colonyops/hive"
`)

	resolved, err := h.RunJSON("config")
	require.NoError(t, err)

	sourceConfig, ok := resolved["sources"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(17), sourceConfig["search_limit"])
	assert.Equal(t, float64(2*time.Minute), sourceConfig["cache_ttl"])

	views, ok := sourceConfig["views"].([]any)
	require.True(t, ok)
	require.Len(t, views, 2)

	global, ok := views[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "my-review-queue", global["name"])
	assert.Equal(t, "prs", global["base"])
	assert.Equal(t, "review-requested:@me state:open", global["query"])
	assert.NotContains(t, global, "scope")

	scoped, ok := views[1].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "triage", scoped["name"])
	assert.Equal(t, "issues", scoped["base"])
	assert.Equal(t, "label:triage no:assignee archived:false", scoped["query"])
	assert.Equal(t, "colonyops/hive", scoped["scope"])
}
