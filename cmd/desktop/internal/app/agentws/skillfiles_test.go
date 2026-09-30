package agentws

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadSharedSkillsReadsSlugsAndDescriptions(t *testing.T) {
	t.Parallel()

	dir := SharedSkillsDir(t.TempDir())
	writeFile(t, filepath.Join(dir, "release-notes", skillFileName),
		"---\nname: release-notes\ndescription: \"Draft release notes: from the merged PRs.\"\n---\n# Release notes\n")
	writeFile(t, filepath.Join(dir, "bare", skillFileName), "# Bare\n\nNo frontmatter here.\n")

	skills, err := LoadSharedSkills(dir)
	require.NoError(t, err)
	require.Len(t, skills, 2)

	assert.Equal(t, "bare", skills[0].Slug, "entries are sorted by slug")
	assert.Empty(t, skills[0].Description, "a file with no frontmatter still loads, without a description")
	assert.Equal(t, "# Bare\n\nNo frontmatter here.\n", skills[0].Body)

	assert.Equal(t, "release-notes", skills[1].Slug)
	assert.Equal(t, "Draft release notes: from the merged PRs.", skills[1].Description)
	assert.Contains(t, skills[1].Body, "---\nname: release-notes", "the body installs verbatim, frontmatter included")
}

// TestLoadSharedSkillsTolerates covers what a directory a user drops files
// into actually contains: entries that are not installable skills are skipped
// rather than failing the read every workspace open depends on.
func TestLoadSharedSkillsTolerates(t *testing.T) {
	t.Parallel()

	t.Run("MissingDirectoryIsEmpty", func(t *testing.T) {
		t.Parallel()
		skills, err := LoadSharedSkills(SharedSkillsDir(t.TempDir()))
		require.NoError(t, err)
		assert.Empty(t, skills)
	})

	t.Run("SkipsEntriesThatCannotBeInstalled", func(t *testing.T) {
		t.Parallel()
		dir := SharedSkillsDir(t.TempDir())
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "no-skill-file"), 0o700))
		writeFile(t, filepath.Join(dir, "loose.md"), "# not a skill directory\n")
		writeFile(t, filepath.Join(dir, "good", skillFileName), "# Good\n")

		skills, err := LoadSharedSkills(dir)
		require.NoError(t, err)
		require.Len(t, skills, 1)
		assert.Equal(t, "good", skills[0].Slug)
	})
}
