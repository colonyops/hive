package feed

import (
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
		{name: "valid", yaml: `profiles:
  - id: work
    name: Work
    feeds:
      - id: prs
        name: PRs
        kind: search
        query: "is:open is:pr author:@me"
      - id: inbox
        name: Inbox
        kind: notifications
        repos: ["acme/*"]
        exclude_repos: ["acme/noisy"]
`},
		{name: "unknown field", wantErr: "not found", yaml: `profiles:
  - id: work
    name: Work
    feeds:
      - id: prs
        name: PRs
        kind: search
        query: "is:pr"
        filter: "acme/*"
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
		{name: "duplicate feed id", wantErr: "defined twice", yaml: `profiles:
  - id: work
    name: Work
    feeds:
      - {id: a, name: A, kind: notifications}
      - {id: a, name: B, kind: notifications}
`},
		{name: "search without query", wantErr: "requires a query", yaml: `profiles:
  - id: work
    name: Work
    feeds:
      - {id: a, name: A, kind: search}
`},
		{name: "notifications with query", wantErr: "takes no query", yaml: `profiles:
  - id: work
    name: Work
    feeds:
      - {id: a, name: A, kind: notifications, query: "is:pr"}
`},
		{name: "unknown kind", wantErr: "unknown kind", yaml: `profiles:
  - id: work
    name: Work
    feeds:
      - {id: a, name: A, kind: rss}
`},
		{name: "bad glob", wantErr: "invalid repo glob", yaml: `profiles:
  - id: work
    name: Work
    feeds:
      - {id: a, name: A, kind: notifications, repos: ["acme/["]}
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

func TestParseConfigFeedCap(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	b.WriteString("profiles:\n  - id: big\n    name: Big\n    feeds:\n")
	for i := 0; i <= maxFeeds; i++ {
		b.WriteString("      - {id: f")
		b.WriteByte(byte('a' + i/10))
		b.WriteByte(byte('0' + i%10))
		b.WriteString(", name: F, kind: notifications}\n")
	}
	_, err := parseConfig([]byte(b.String()))
	require.ErrorContains(t, err, "rate limit")
}

func TestFeedDefMatchesRepo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		def  FeedDef
		repo string
		want bool
	}{
		{name: "no filters includes all", def: FeedDef{}, repo: "acme/app", want: true},
		{name: "include glob match", def: FeedDef{Repos: []string{"acme/*"}}, repo: "acme/app", want: true},
		{name: "include glob miss", def: FeedDef{Repos: []string{"acme/*"}}, repo: "other/app", want: false},
		{name: "glob does not cross slash", def: FeedDef{Repos: []string{"*"}}, repo: "acme/app", want: false},
		{name: "doublestar crosses slash", def: FeedDef{Repos: []string{"**"}}, repo: "acme/app", want: true},
		{name: "exclude wins over include", def: FeedDef{Repos: []string{"acme/*"}, ExcludeRepos: []string{"acme/noisy"}}, repo: "acme/noisy", want: false},
		{name: "exclude only", def: FeedDef{ExcludeRepos: []string{"acme/noisy"}}, repo: "acme/app", want: true},
		{name: "alternation", def: FeedDef{Repos: []string{"acme/{app,web}"}}, repo: "acme/web", want: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, tc.def.matchesRepo(tc.repo))
		})
	}
}

func TestExampleConfigParses(t *testing.T) {
	t.Parallel()

	profiles, err := parseConfig([]byte(ExampleConfig()))
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	assert.Equal(t, "triage", profiles[0].ID)
}
