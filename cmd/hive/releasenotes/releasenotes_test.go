package releasenotes

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/releasenotes"
)

// TestChangelogParses keeps a malformed entry or fragment from shipping.
func TestChangelogParses(t *testing.T) {
	entries, err := releasenotes.Load(Changelog)
	require.NoError(t, err)

	for _, entry := range entries {
		if entry.Draft {
			continue
		}
		assert.False(t, entry.Date.IsZero(), "%s has no date", entry.Version)
		assert.NotEmpty(t, entry.Summary+entry.Body, "%s says nothing", entry.Version)
	}
}
