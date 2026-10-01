package wailsui

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
)

// TestNodeTypesMatchPipelineNodeDirectories is the bijection between the Go
// node registry and the frontend's per-type editor directories. A node type
// registered in Go with no matching cmd/desktop/frontend/src/pipeline/nodes/<type>
// directory has no palette entry, drawer, or editor — it fails silently,
// nothing else in the build catches it, and the seven type strings hardcoded
// in the pipeline's node-loading spec do not notice either. Same pattern as
// TestAppErrorKindUnionMatchesTheCoreKinds (errors_test.go): a Go-side
// enumeration checked against a plain read of the frontend source tree.
func TestNodeTypesMatchPipelineNodeDirectories(t *testing.T) {
	t.Parallel()

	const dir = "../../../frontend/src/pipeline/nodes"
	entries, err := os.ReadDir(dir)
	require.NoErrorf(t, err, "reading %s", dir)

	frontend := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if len(name) > 0 && name[0] == '.' {
			continue // dotfiles (.DS_Store, editor swap dirs, ...)
		}
		if !entry.IsDir() {
			continue // e.g. a stray file directly under nodes/
		}
		frontend = append(frontend, name)
	}

	backend := flow.NodeTypes()

	backendSet := make(map[string]bool, len(backend))
	for _, nodeType := range backend {
		backendSet[nodeType] = true
	}
	frontendSet := make(map[string]bool, len(frontend))
	for _, name := range frontend {
		frontendSet[name] = true
	}

	var missingFrontendDir []string
	for _, nodeType := range backend {
		if !frontendSet[nodeType] {
			missingFrontendDir = append(missingFrontendDir, nodeType)
		}
	}
	var missingBackendType []string
	for _, name := range frontend {
		if !backendSet[name] {
			missingBackendType = append(missingBackendType, name)
		}
	}

	assert.Emptyf(t, missingFrontendDir,
		"flow.NodeTypes() has types with no matching directory under %s: %v", dir, missingFrontendDir)
	assert.Emptyf(t, missingBackendType,
		"%s has directories with no matching flow.NodeTypes() entry: %v", dir, missingBackendType)
}
