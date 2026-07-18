package feed

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// filterItem builds a liveItem with the fields filtering inspects.
func filterItem(id, kind, repo, author, reason string, labels ...string) liveItem {
	return liveItem{item: Item{ID: id, Kind: kind, Repo: repo, Author: author, Reason: reason, Labels: labels}}
}

func TestApplyFilters(t *testing.T) {
	t.Parallel()

	searchPR := filterItem("acme/app#1", "PR", "acme/app", "hayden", "", "bug", "area/ui")
	searchIssue := filterItem("acme/web#2", "Issue", "acme/web", "Mira", "", "wontfix")
	botPR := filterItem("acme/app#3", "PR", "acme/app", "dependabot[bot]", "")
	notifIssue := filterItem("other/repo#4", "Issue", "other/repo", "", "mention")
	mergedPR := filterItem("acme/app#5", "PR", "acme/app", "koji", "review_requested", "bug")
	all := []liveItem{searchPR, searchIssue, botPR, notifIssue, mergedPR}

	ids := func(items []liveItem) []string {
		out := make([]string, 0, len(items))
		for _, li := range items {
			out = append(out, li.item.ID)
		}
		return out
	}

	tests := []struct {
		name    string
		filters FilterDef
		want    []string
	}{
		{name: "no filters keeps all", filters: FilterDef{}, want: ids(all)},
		{name: "repos include", filters: FilterDef{Repos: []string{"acme/*"}}, want: []string{"acme/app#1", "acme/web#2", "acme/app#3", "acme/app#5"}},
		{name: "repos exclude", filters: FilterDef{ExcludeRepos: []string{"acme/web"}}, want: []string{"acme/app#1", "acme/app#3", "other/repo#4", "acme/app#5"}},
		{name: "exclude wins over include", filters: FilterDef{Repos: []string{"acme/*"}, ExcludeRepos: []string{"acme/app"}}, want: []string{"acme/web#2"}},
		{name: "authors case-insensitive", filters: FilterDef{Authors: []string{"MIRA"}}, want: []string{"acme/web#2"}},
		{name: "authors glob", filters: FilterDef{Authors: []string{"h*"}}, want: []string{"acme/app#1"}},
		{name: "exclude bot author literal brackets", filters: FilterDef{ExcludeAuthors: []string{"dependabot[bot]"}}, want: []string{"acme/app#1", "acme/web#2", "other/repo#4", "acme/app#5"}},
		{name: "exclude bot author wildcard", filters: FilterDef{ExcludeAuthors: []string{"*[bot]"}}, want: []string{"acme/app#1", "acme/web#2", "other/repo#4", "acme/app#5"}},
		{name: "labels any-match", filters: FilterDef{Labels: []string{"area/*"}}, want: []string{"acme/app#1"}},
		{name: "labels or within group", filters: FilterDef{Labels: []string{"bug", "wontfix"}}, want: []string{"acme/app#1", "acme/web#2", "acme/app#5"}},
		{name: "exclude labels", filters: FilterDef{ExcludeLabels: []string{"wontfix"}}, want: []string{"acme/app#1", "acme/app#3", "other/repo#4", "acme/app#5"}},
		{name: "types pr", filters: FilterDef{Types: []string{"pr"}}, want: []string{"acme/app#1", "acme/app#3", "acme/app#5"}},
		{name: "types issue case-insensitive", filters: FilterDef{Types: []string{"ISSUE"}}, want: []string{"acme/web#2", "other/repo#4"}},
		{name: "reasons match", filters: FilterDef{Reasons: []string{"mention", "review_requested"}}, want: []string{"other/repo#4", "acme/app#5"}},
		{name: "reasons exclude search-only items", filters: FilterDef{Reasons: []string{"mention"}}, want: []string{"other/repo#4"}},
		{name: "groups AND together", filters: FilterDef{Repos: []string{"acme/*"}, Types: []string{"pr"}, Labels: []string{"bug"}}, want: []string{"acme/app#1", "acme/app#5"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := applyFilters(FeedDef{Filters: tc.filters}, all)
			assert.Equal(t, tc.want, ids(got))
		})
	}
}

func TestFilterDefIsZero(t *testing.T) {
	t.Parallel()

	assert.True(t, FilterDef{}.IsZero())
	assert.False(t, FilterDef{Types: []string{"pr"}}.IsZero())
	assert.False(t, FilterDef{ExcludeAuthors: []string{"*[bot]"}}.IsZero())
}
