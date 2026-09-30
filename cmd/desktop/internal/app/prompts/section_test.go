package prompts

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSectionRewritesDocOutlineToFit(t *testing.T) {
	doc := "# GitHub source\n\nProse.\n\n## Fields\n\n- `kind`\n\n### Detail\n\nMore.\n"
	got := section(4, "sources.github", doc)

	assert.Contains(t, got, "#### GitHub source — `sources.github`")
	assert.Contains(t, got, "##### Fields")
	assert.Contains(t, got, "###### Detail")
	assert.NotContains(t, got, "\n# ")
}

// TestSectionLeavesFencedCodeAlone — a `#` opening a line inside a fenced block
// is a shell comment or a YAML comment, and rewriting it would corrupt the
// example an agent is meant to copy.
func TestSectionLeavesFencedCodeAlone(t *testing.T) {
	doc := "# Shell\n\n```sh\n# not a heading\necho hi\n```\n\n## After\n"
	got := section(3, "shell", doc)

	assert.Contains(t, got, "\n# not a heading\n")
	assert.Contains(t, got, "### Shell — `shell`")
	assert.Contains(t, got, "#### After")
}

func TestSectionClampsAtMaxHeadingLevel(t *testing.T) {
	got := section(5, "feed", "# Feed\n\n### Deep\n")
	assert.Contains(t, got, "##### Feed — `feed`")
	// 3 + 4 would be 7; markdown stops at 6.
	assert.Contains(t, got, "###### Deep")
	assert.NotContains(t, got, "#######")
}

// TestSectionIgnoresNonHeadingHashes — "#42" is an issue reference, not an H1.
func TestSectionIgnoresNonHeadingHashes(t *testing.T) {
	got := section(3, "feed", "# Feed\n\nSee #42 for context.\n")
	assert.Contains(t, got, "See #42 for context.")
}
