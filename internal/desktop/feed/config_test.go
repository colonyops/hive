package feed

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseConfigStrict(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{name: "empty file", yaml: ""},
		{name: "valid", yaml: `sources:
  - id: my-prs
    kind: search
    query: "is:open is:pr author:@me"
    limit: 100
  - id: inbox
    kind: notifications
profiles:
  - id: work
    name: Work
    feeds:
      - id: prs
        name: PRs
        sources: [my-prs]
      - id: triage
        name: Triage
        sources: [my-prs, inbox]
        filters:
          repos: ["acme/*"]
          exclude_repos: ["acme/noisy"]
          authors: ["hayden"]
          exclude_authors: ["*[bot]"]
          labels: ["bug", "area/*"]
          exclude_labels: ["wontfix"]
          types: [pr, issue]
          reasons: [mention, review_requested]
`},
		{name: "unknown field", wantErr: "not found", yaml: `sources:
  - id: s
    kind: notifications
    color: red
`},
		{name: "legacy feed kind gets hint", wantErr: "top-level sources", yaml: `profiles:
  - id: work
    name: Work
    feeds:
      - id: prs
        name: PRs
        kind: search
        query: "is:pr"
`},
		{name: "legacy feed repos gets hint", wantErr: "filters", yaml: `profiles:
  - id: work
    name: Work
    feeds:
      - id: prs
        name: PRs
        repos: ["acme/*"]
`},
		{name: "missing source id", wantErr: "id is required", yaml: `sources:
  - kind: notifications
`},
		{name: "duplicate source id", wantErr: "defined twice", yaml: `sources:
  - {id: s, kind: notifications}
  - {id: s, kind: search, query: "is:pr"}
`},
		{name: "search source without query", wantErr: "requires a query", yaml: `sources:
  - {id: s, kind: search}
`},
		{name: "notifications source with query", wantErr: "takes no query", yaml: `sources:
  - {id: s, kind: notifications, query: "is:pr"}
`},
		{name: "unknown source kind", wantErr: "unknown kind", yaml: `sources:
  - {id: s, kind: rss}
`},
		{name: "negative limit", wantErr: "negative", yaml: `sources:
  - {id: s, kind: notifications, limit: -1}
`},
		{name: "search limit above cap", wantErr: "page cap of 100", yaml: `sources:
  - {id: s, kind: search, query: "is:pr", limit: 101}
`},
		{name: "notifications limit above cap", wantErr: "page cap of 50", yaml: `sources:
  - {id: s, kind: notifications, limit: 51}
`},
		{name: "missing profile id", wantErr: "id is required", yaml: `profiles:
  - name: Work
    feeds: []
`},
		{name: "duplicate profile id", wantErr: "defined twice", yaml: `profiles:
  - id: work
    name: Work
    feeds: []
  - id: work
    name: Other
    feeds: []
`},
		{name: "duplicate feed id", wantErr: "defined twice", yaml: `sources:
  - {id: s, kind: notifications}
profiles:
  - id: work
    name: Work
    feeds:
      - {id: a, name: A, sources: [s]}
      - {id: a, name: B, sources: [s]}
`},
		{name: "feed without sources", wantErr: "at least one source", yaml: `profiles:
  - id: work
    name: Work
    feeds:
      - {id: a, name: A}
`},
		{name: "feed with unknown source", wantErr: "unknown source", yaml: `profiles:
  - id: work
    name: Work
    feeds:
      - {id: a, name: A, sources: [ghost]}
`},
		{name: "bad repo glob", wantErr: "invalid repos glob", yaml: `sources:
  - {id: s, kind: notifications}
profiles:
  - id: work
    name: Work
    feeds:
      - id: a
        name: A
        sources: [s]
        filters:
          repos: ["acme/["]
`},
		{name: "bad exclude author glob", wantErr: "invalid exclude_authors glob", yaml: `sources:
  - {id: s, kind: notifications}
profiles:
  - id: work
    name: Work
    feeds:
      - id: a
        name: A
        sources: [s]
        filters:
          exclude_authors: ["{a"]
`},
		{name: "unknown type", wantErr: "unknown type", yaml: `sources:
  - {id: s, kind: notifications}
profiles:
  - id: work
    name: Work
    feeds:
      - id: a
        name: A
        sources: [s]
        filters:
          types: [discussion]
`},
		{name: "unknown reason", wantErr: "unknown notification reason", yaml: `sources:
  - {id: s, kind: notifications}
profiles:
  - id: work
    name: Work
    feeds:
      - id: a
        name: A
        sources: [s]
        filters:
          reasons: [poked]
`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseConfig([]byte(tc.yaml))
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestParseConfigSearchSourceCap(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	b.WriteString("sources:\n")
	for i := 0; i <= maxSearchSources; i++ {
		fmt.Fprintf(&b, "  - {id: s%d, kind: search, query: \"is:pr\"}\n", i)
	}
	_, err := parseConfig([]byte(b.String()))
	require.ErrorContains(t, err, "rate limit")
	require.ErrorContains(t, err, "search")

	// Notifications sources are not capped: the same count of them passes.
	b.Reset()
	b.WriteString("sources:\n")
	for i := 0; i <= maxSearchSources; i++ {
		fmt.Fprintf(&b, "  - {id: n%d, kind: notifications}\n", i)
	}
	_, err = parseConfig([]byte(b.String()))
	require.NoError(t, err)
}

func TestSourceDefEffectiveLimit(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 50, SourceDef{Kind: "search"}.effectiveLimit())
	assert.Equal(t, 100, SourceDef{Kind: "search", Limit: 100}.effectiveLimit())
	assert.Equal(t, 50, SourceDef{Kind: "notifications"}.effectiveLimit())
	assert.Equal(t, 10, SourceDef{Kind: "notifications", Limit: 10}.effectiveLimit())
}

func TestAppendFeedToConfigPreservesComments(t *testing.T) {
	t.Parallel()

	data := []byte(`# my dotfiles-managed feeds
sources:
  - id: inbox
    kind: notifications
profiles:
  # the day job
  - id: work
    name: Work
    feeds:
      - id: prs
        name: My PRs
        sources: [inbox]
`)
	updated, err := appendFeedToConfig(data, "work", FeedDef{ID: "inbox-feed", Name: "Inbox", Sources: []string{"inbox"}})
	require.NoError(t, err)
	assert.Contains(t, string(updated), "# my dotfiles-managed feeds")
	assert.Contains(t, string(updated), "# the day job")
	assert.Contains(t, string(updated), "inbox-feed")

	file, err := parseConfig(updated)
	require.NoError(t, err)
	require.Len(t, file.Profiles[0].Feeds, 2)
	assert.Equal(t, "inbox-feed", file.Profiles[0].Feeds[1].ID)
}

func TestAppendFeedToConfigNullFeeds(t *testing.T) {
	t.Parallel()

	data := []byte(`sources:
  - id: inbox
    kind: notifications
profiles:
  - id: work
    name: Work
    feeds:
`)
	updated, err := appendFeedToConfig(data, "work", FeedDef{ID: "a", Name: "A", Sources: []string{"inbox"}})
	require.NoError(t, err)
	file, err := parseConfig(updated)
	require.NoError(t, err)
	require.Len(t, file.Profiles[0].Feeds, 1)

	_, err = appendFeedToConfig(data, "ghost", FeedDef{ID: "a", Name: "A", Sources: []string{"inbox"}})
	require.ErrorContains(t, err, "not found")
}

func TestUpdateFeedInConfig(t *testing.T) {
	t.Parallel()

	data := []byte(`# header comment
sources:
  - id: inbox
    kind: notifications
profiles:
  - id: work
    name: Work
    feeds:
      - id: a
        name: A
        sources: [inbox]
      - id: b
        name: B
        sources: [inbox]
`)
	def := FeedDef{ID: "a", Name: "Renamed", Sources: []string{"inbox"}, Filters: FilterDef{Reasons: []string{"mention"}}}
	updated, err := updateFeedInConfig(data, "work", "a", def)
	require.NoError(t, err)
	assert.Contains(t, string(updated), "# header comment")

	file, err := parseConfig(updated)
	require.NoError(t, err)
	require.Len(t, file.Profiles[0].Feeds, 2)
	assert.Equal(t, "Renamed", file.Profiles[0].Feeds[0].Name)
	assert.Equal(t, []string{"mention"}, file.Profiles[0].Feeds[0].Filters.Reasons)
	assert.Equal(t, "B", file.Profiles[0].Feeds[1].Name, "sibling feed untouched")

	_, err = updateFeedInConfig(data, "work", "ghost", def)
	require.ErrorContains(t, err, "not found")
}

func TestDeleteFeedFromConfig(t *testing.T) {
	t.Parallel()

	data := []byte(`# header comment
sources:
  - id: inbox
    kind: notifications
profiles:
  # the day job
  - id: work
    name: Work
    feeds:
      - id: a
        name: A
        sources: [inbox]
      # keep this one, it matters
      - id: b
        name: B
        sources: [inbox]
      - id: c
        name: C
        sources: [inbox]
`)
	// Delete a feed from the middle of the list; comments elsewhere survive.
	updated, err := deleteFeedFromConfig(data, "work", "b")
	require.NoError(t, err)
	assert.Contains(t, string(updated), "# header comment")
	assert.Contains(t, string(updated), "# the day job")

	file, err := parseConfig(updated)
	require.NoError(t, err)
	require.Len(t, file.Profiles[0].Feeds, 2)
	assert.Equal(t, "a", file.Profiles[0].Feeds[0].ID)
	assert.Equal(t, "c", file.Profiles[0].Feeds[1].ID)

	// Deleting the only remaining feed leaves an empty feeds sequence.
	onlyOne := []byte(`sources:
  - id: inbox
    kind: notifications
profiles:
  - id: work
    name: Work
    feeds:
      - id: solo
        name: Solo
        sources: [inbox]
`)
	updated, err = deleteFeedFromConfig(onlyOne, "work", "solo")
	require.NoError(t, err)
	file, err = parseConfig(updated)
	require.NoError(t, err)
	assert.Empty(t, file.Profiles[0].Feeds)

	_, err = deleteFeedFromConfig(data, "work", "ghost")
	require.ErrorContains(t, err, "not found")
	_, err = deleteFeedFromConfig(data, "ghost", "a")
	require.ErrorContains(t, err, "not found")
}

func TestDeleteProfileFromConfig(t *testing.T) {
	t.Parallel()

	data := []byte(`# my dotfiles-managed feeds
sources:
  - id: inbox
    kind: notifications
profiles:
  # the day job
  - id: work
    name: Work
    feeds:
      - id: a
        name: A
        sources: [inbox]
  - id: side
    name: Side Projects
    feeds:
      - id: b
        name: B
        sources: [inbox]
`)
	updated, err := deleteProfileFromConfig(data, "work")
	require.NoError(t, err)
	assert.Contains(t, string(updated), "# my dotfiles-managed feeds")
	assert.NotContains(t, string(updated), "# the day job", "comment attached to the removed profile is gone with it")

	file, err := parseConfig(updated)
	require.NoError(t, err)
	require.Len(t, file.Profiles, 1)
	assert.Equal(t, "side", file.Profiles[0].ID)
	// Sources are untouched by a profile deletion.
	require.Len(t, file.Sources, 1)
	assert.Equal(t, "inbox", file.Sources[0].ID)

	// Deleting the last remaining profile leaves an empty (but valid) profiles
	// list; sources stay behind since they are decoupled and shared.
	updated, err = deleteProfileFromConfig(updated, "side")
	require.NoError(t, err)
	file, err = parseConfig(updated)
	require.NoError(t, err)
	assert.Empty(t, file.Profiles)
	require.Len(t, file.Sources, 1)

	_, err = deleteProfileFromConfig(data, "ghost")
	require.ErrorContains(t, err, "not found")
}

func TestAppendSourceToConfigCreatesKey(t *testing.T) {
	t.Parallel()

	// A profiles-only document gains a sources key.
	data := []byte("profiles: []\n")
	updated, err := appendSourceToConfig(data, SourceDef{ID: "inbox", Kind: "notifications"})
	require.NoError(t, err)
	file, err := parseConfig(updated)
	require.NoError(t, err)
	require.Len(t, file.Sources, 1)
	assert.Equal(t, "inbox", file.Sources[0].ID)

	// An empty document gets the header and the key.
	updated, err = appendSourceToConfig(nil, SourceDef{ID: "inbox", Kind: "notifications"})
	require.NoError(t, err)
	assert.Contains(t, string(updated), "# Hive Desktop feeds")
	file, err = parseConfig(updated)
	require.NoError(t, err)
	require.Len(t, file.Sources, 1)
}

func TestExampleConfigParses(t *testing.T) {
	t.Parallel()

	file, err := parseConfig([]byte(ExampleConfig()))
	require.NoError(t, err)
	require.Len(t, file.Sources, 3)
	require.Len(t, file.Profiles, 1)
	assert.Equal(t, "triage", file.Profiles[0].ID)
	require.Len(t, file.Profiles[0].Feeds, 3)
	assert.Equal(t, "my-open-prs", file.Profiles[0].Feeds[0].ID, "e2e asserts on this feed id")
	assert.Equal(t, []string{"dependabot[bot]"}, file.Profiles[0].Feeds[1].Filters.ExcludeAuthors)
}
