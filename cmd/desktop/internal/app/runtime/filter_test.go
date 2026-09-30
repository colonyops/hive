package runtime

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
)

// The glob dialect is small and unusual, and it is the shipped behaviour
// rather than doublestar's. Every row here is a case where a general-purpose
// matcher would answer differently.
func TestGlobDialect(t *testing.T) {
	t.Parallel()

	cases := []struct {
		pattern string
		value   string
		want    bool
		why     string
	}{
		{"acme/*", "acme/app", true, "a single star fills one segment"},
		{"acme/*", "acme/sub/app", false, "a single star does not cross a slash"},
		{"acme/**", "acme/sub/app", true, "a double star crosses slashes"},
		{"**", "anything/at/all", true, "a bare double star matches everything"},
		{"*[bot]", "dependabot[bot]", true, "brackets are literal, not a character class"},
		{"*[bot]", "dependabotb", false, "…so a single character inside them does not match"},
		{"a?c", "a?c", true, "a question mark is literal, not a single-character wildcard"},
		{"a?c", "abc", false, "…so it does not match another character"},
		{"{a,b}", "{a,b}", true, "braces are literal, not alternation"},
		{"{a,b}", "a", false, "…so neither branch matches on its own"},
		{"a.c", "abc", false, "a dot is literal, not any-character"},
		{"exact", "exact", true, "an unanchored-looking pattern is still fully anchored"},
		{"exact", "inexact", false, "…at the start"},
		{"exact", "exactly", false, "…and at the end"},
	}

	for _, tc := range cases {
		t.Run(tc.pattern+" vs "+tc.value, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, globToRegexp(tc.pattern).MatchString(tc.value), tc.why)
		})
	}
}

func TestFilterRuleComposition(t *testing.T) {
	t.Parallel()

	cfg := &flow.GithubFilterConfig{
		Repos:         []string{"acme/*", "other/one"},
		ExcludeLabels: []string{"wip"},
		Types:         []string{"PR"},
	}
	p := newFilter(t, cfg)

	require.True(t, p.matches(filterableItem{Repo: "acme/app", Kind: "pr"}), "groups AND, values within a group OR")
	require.True(t, p.matches(filterableItem{Repo: "other/one", Kind: "pr"}), "the second value of the repos group also passes")
	require.False(t, p.matches(filterableItem{Repo: "third/one", Kind: "pr"}), "no value in the repos group matches")
	require.False(t, p.matches(filterableItem{Repo: "acme/app", Kind: "issue"}), "the types group excludes it")
	require.False(t, p.matches(filterableItem{Repo: "acme/app", Kind: "pr", Labels: []string{"ready", "wip"}}), "an exclude group wins over an include")
	require.True(t, p.matches(filterableItem{Repo: "acme/app", Kind: "PR"}), "type comparison folds case")
}

func TestFilterPullRequestStatus(t *testing.T) {
	t.Parallel()

	p := newFilter(t, &flow.GithubFilterConfig{CI: []string{"passing"}, Review: []string{"approved"}})
	require.True(t, p.matches(filterableItem{Kind: "PR", CI: "PASSING", Review: "approved"}))
	require.False(t, p.matches(filterableItem{Kind: "PR", CI: "failing", Review: "approved"}))
	require.False(t, p.matches(filterableItem{Kind: "PR", CI: "passing", Review: "changes_requested"}))
	require.False(t, p.matches(filterableItem{Kind: "Issue"}), "missing PR fields must not match")
}

func TestFilterAuthorsFoldCase(t *testing.T) {
	t.Parallel()

	p := newFilter(t, &flow.GithubFilterConfig{Authors: []string{"Alice"}})
	require.True(t, p.matches(filterableItem{Author: "alice"}))
	require.True(t, p.matches(filterableItem{Author: "ALICE"}))
	require.False(t, p.matches(filterableItem{Author: "bob"}))
}

// An item with no reason is search-only. A reasons filter is asking for
// notification-derived items, so it must exclude those rather than treat the
// missing value as a wildcard.
func TestFilterReasonsExcludeItemsWithoutOne(t *testing.T) {
	t.Parallel()

	p := newFilter(t, &flow.GithubFilterConfig{Reasons: []string{"mention"}})
	require.True(t, p.matches(filterableItem{Reason: "mention"}))
	require.False(t, p.matches(filterableItem{Reason: ""}))
}

func newFilter(t *testing.T, cfg *flow.GithubFilterConfig) *filterProcessor {
	t.Helper()
	p, err := newFilterNode(nil, "n", cfg)
	require.NoError(t, err)
	fp, ok := p.(*filterProcessor)
	require.True(t, ok, "newFilterNode should return a *filterProcessor")
	return fp
}
