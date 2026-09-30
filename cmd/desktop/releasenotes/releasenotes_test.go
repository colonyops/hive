package releasenotes

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestChangelogParses is the gate that keeps a malformed entry from shipping:
// every committed release entry must have a well-formed header whose version
// matches its filename, every unreleased fragment must parse, and there is at
// most one draft.
func TestChangelogParses(t *testing.T) {
	entries, err := Load()
	require.NoError(t, err)

	drafts := 0
	seen := make(map[string]bool, len(entries))
	for _, entry := range entries {
		if entry.Draft {
			drafts++
			assert.Empty(t, entry.Version, "the draft describes no version")
			continue
		}
		assert.False(t, seen[entry.Version], "duplicate entry for %s", entry.Version)
		seen[entry.Version] = true
		assert.False(t, entry.Date.IsZero(), "%s has no date", entry.Version)
		assert.NotEmpty(t, entry.Body, "%s has an empty body", entry.Version)
	}
	assert.LessOrEqual(t, drafts, 1, "only one entry may be the draft")
}

func TestChangelogIsOrderedNewestFirst(t *testing.T) {
	entries, err := Load()
	require.NoError(t, err)

	for i := 1; i < len(entries); i++ {
		assert.Positive(t, compareEntries(entries[i-1], entries[i]),
			"%s should sort above %s", entries[i-1].Version, entries[i].Version)
	}
}

func entries(versions ...string) Entries {
	out := make(Entries, 0, len(versions))
	for _, v := range versions {
		out = append(out, Entry{Version: v, Body: "notes for " + v})
	}
	return out
}

func draft() Entry { return Entry{Draft: true, Body: "unreleased work"} }

func versionsOf(e Entries) []string {
	out := make([]string, 0, len(e))
	for _, entry := range e {
		if entry.Draft {
			out = append(out, "draft")
			continue
		}
		out = append(out, entry.Version)
	}
	return out
}

func TestBetweenReturnsEveryReleaseCrossed(t *testing.T) {
	all := entries("1.4.0", "1.3.0", "1.2.0", "1.1.0")

	assert.Equal(t, []string{"1.4.0", "1.3.0"},
		versionsOf(all.Between("1.2.0", "1.4.0")),
		"a user who skipped a release sees both sets of notes they crossed")

	assert.Empty(t, all.Between("1.4.0", "1.4.0"),
		"the version already acknowledged is not shown again")
	assert.Empty(t, all.Between("1.4.0", "1.2.0"), "a downgrade shows nothing")
}

// The draft is what this build has that no stable release does, so every
// upgrade that reaches it shows it — including a prerelease bump, which is the
// only thing such a bump has to say.
func TestBetweenAlwaysIncludesTheDraft(t *testing.T) {
	all := Entries{draft(), Entry{Version: "1.2.0", Body: "notes for 1.2.0"}}

	assert.Equal(t, []string{"draft"}, versionsOf(all.Between("1.3.0-dev.3", "1.3.0-dev.4")))
	assert.Equal(t, []string{"draft"}, versionsOf(all.Between("1.2.0", "1.3.0-beta.1")))
	assert.Equal(t, []string{"draft", "1.2.0"}, versionsOf(all.Between("1.1.0", "1.3.0-dev.1")))
}

// A release still to come is not this build's to describe: a binary can only
// carry notes up to its own version.
func TestBetweenExcludesReleasesAboveTheRunningVersion(t *testing.T) {
	all := entries("1.4.0", "1.3.0", "1.2.0")

	assert.Equal(t, []string{"1.3.0"}, versionsOf(all.Between("1.2.0", "1.3.0")))
}

func TestBetweenIgnoresUnpublishableVersions(t *testing.T) {
	all := Entries{draft(), Entry{Version: "1.2.0", Body: "notes for 1.2.0"}}

	assert.Empty(t, all.Between("1.2.0", "dev"), "a source build has no notes")
	assert.Equal(t, []string{"draft", "1.2.0"},
		versionsOf(all.Between("", "1.3.0-dev.1")),
		"no recorded version means no lower bound")
}

// Promotion runs dev -> beta -> stable within a base version, which is the
// reverse of how semver orders the words "beta" and "dev".
func TestOrderingFollowsThePromotionPath(t *testing.T) {
	assert.True(t, IsNewer("1.2.0-beta.1", "1.2.0-dev.9"))
	assert.True(t, IsNewer("1.2.0", "1.2.0-beta.9"))
	assert.True(t, IsNewer("1.2.1-dev.1", "1.2.0"))
	assert.False(t, IsNewer("1.2.0-dev.9", "1.2.0-beta.1"))
	assert.False(t, IsNewer("dev", "1.2.0"), "a source build never counts as an upgrade")
	assert.True(t, IsNewer("1.2.0", "(devel)"), "nothing recorded means anything published is newer")
}

func TestIsPublishedRejectsUnreleasedVersions(t *testing.T) {
	for _, version := range []string{"dev", "(devel)", "v0.0.0-20260101000000-abcdef123456", "1.2.0-rc.1", ""} {
		assert.False(t, IsPublished(version), "%q must not count as published", version)
	}
	for _, version := range []string{"1.2.0", "v1.2.0", "desktop-v1.2.0", "1.2.0-beta.2", "1.2.0-dev.7"} {
		assert.True(t, IsPublished(version), "%q should count as published", version)
	}
}

func TestFind(t *testing.T) {
	all := Entries{draft(), Entry{Version: "1.2.0", Body: "notes for 1.2.0"}}

	entry, ok := all.Find("v1.2.0")
	require.True(t, ok, "the v prefix is accepted")
	assert.Equal(t, "1.2.0", entry.Version)

	_, ok = all.Find("9.9.9")
	assert.False(t, ok, "the draft is not a match for a version it has not been promoted into")
}

func TestParseEntryRejectsMismatchedFilename(t *testing.T) {
	_, err := parseEntry("1.2.0.md", []byte("---\nversion: 1.2.1\ndate: 2026-01-01\n---\n\nnotes\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match its filename")
}

// Only a stable release gets an entry of its own; a prerelease's notes live in
// the draft, which is what keeps the read path free of channel filtering.
func TestParseEntryRejectsAPrerelease(t *testing.T) {
	_, err := parseEntry("1.2.0-dev.3.md", []byte("---\nversion: 1.2.0-dev.3\ndate: 2026-01-01\n---\n\nnotes\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), UnreleasedDir)
}

func TestParseEntryRequiresAValidHeader(t *testing.T) {
	for name, raw := range map[string]string{
		"no frontmatter": "just a body\n",
		"unterminated":   "---\nversion: 1.2.0\n",
		"bad date":       "---\nversion: 1.2.0\ndate: 06/01/2026\n---\n\nnotes\n",
		"unpublishable":  "---\nversion: 1.2.0-rc.1\ndate: 2026-01-01\n---\n\nnotes\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parseEntry("1.2.0.md", []byte(raw))
			assert.Error(t, err)
		})
	}
}

func TestParseEntryReadsSummaryAndBody(t *testing.T) {
	entry, err := parseEntry("1.2.0.md", []byte(
		"---\nversion: 1.2.0\ndate: 2026-01-02\nsummary: A short line.\n---\n\n## Added\n\n- a thing\n"))
	require.NoError(t, err)

	assert.Equal(t, "1.2.0", entry.Version)
	assert.False(t, entry.Draft)
	assert.Equal(t, "A short line.", entry.Summary)
	assert.Equal(t, "## Added\n\n- a thing", entry.Body)
	assert.Equal(t, 2026, entry.Date.Year())
}
