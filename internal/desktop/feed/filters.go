package feed

import (
	"fmt"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// FilterDef is a feed's client-side filter block. Filters run after the
// feed's source items are merged, so they change what is shown, not what is
// requested — filtering is free of API cost. Groups AND together; values
// within a group OR; exclude groups win over includes.
type FilterDef struct {
	// Repos/ExcludeRepos are doublestar globs matched against "owner/repo"
	// (e.g. "colonyops/*").
	Repos        []string `json:"repos,omitempty"         yaml:"repos,omitempty"`
	ExcludeRepos []string `json:"exclude_repos,omitempty" yaml:"exclude_repos,omitempty"`
	// Authors/ExcludeAuthors are globs matched against the item author
	// case-insensitively, with "[" and "]" taken literally (bot logins end
	// in a literal "[bot]"; a character class over login characters has no
	// realistic use).
	Authors        []string `json:"authors,omitempty"         yaml:"authors,omitempty"`
	ExcludeAuthors []string `json:"exclude_authors,omitempty" yaml:"exclude_authors,omitempty"`
	// Labels keeps items where ANY item label matches ANY glob;
	// ExcludeLabels drops items where any label matches.
	Labels        []string `json:"labels,omitempty"         yaml:"labels,omitempty"`
	ExcludeLabels []string `json:"exclude_labels,omitempty" yaml:"exclude_labels,omitempty"`
	// Types matches the item kind: "pr" or "issue" (case-insensitive).
	Types []string `json:"types,omitempty" yaml:"types,omitempty"`
	// Reasons matches the notification reason. Items without a reason —
	// search results that are not also in the notifications inbox — never
	// match, so a reasons filter excludes them.
	Reasons []string `json:"reasons,omitempty" yaml:"reasons,omitempty"`
}

// IsZero reports whether no filter group is set. yaml.v3 consults it for
// omitempty, so feeds written by the app omit an empty filters block.
func (f FilterDef) IsZero() bool {
	return len(f.Repos) == 0 && len(f.ExcludeRepos) == 0 &&
		len(f.Authors) == 0 && len(f.ExcludeAuthors) == 0 &&
		len(f.Labels) == 0 && len(f.ExcludeLabels) == 0 &&
		len(f.Types) == 0 && len(f.Reasons) == 0
}

// validTypes are the allowed values of the types filter, matched against
// Item.Kind case-insensitively.
var validTypes = map[string]bool{"pr": true, "issue": true}

// validReasons are the notification reasons GitHub delivers.
var validReasons = map[string]bool{
	"approval_requested":       true,
	"assign":                   true,
	"author":                   true,
	"ci_activity":              true,
	"comment":                  true,
	"invitation":               true,
	"manual":                   true,
	"member_feature_requested": true,
	"mention":                  true,
	"review_requested":         true,
	"security_advisory_credit": true,
	"security_alert":           true,
	"state_change":             true,
	"subscribed":               true,
	"team_mention":             true,
}

func validateFilters(f FilterDef) error {
	globGroups := []struct {
		name     string
		patterns []string
	}{
		{"repos", f.Repos},
		{"exclude_repos", f.ExcludeRepos},
		{"authors", f.Authors},
		{"exclude_authors", f.ExcludeAuthors},
		{"labels", f.Labels},
		{"exclude_labels", f.ExcludeLabels},
	}
	for _, group := range globGroups {
		for _, pattern := range group.patterns {
			if !doublestar.ValidatePattern(pattern) {
				return fmt.Errorf("invalid %s glob %q", group.name, pattern)
			}
		}
	}
	for _, t := range f.Types {
		if !validTypes[strings.ToLower(t)] {
			return fmt.Errorf("unknown type %q (want \"pr\" or \"issue\")", t)
		}
	}
	for _, reason := range f.Reasons {
		if !validReasons[reason] {
			return fmt.Errorf("unknown notification reason %q", reason)
		}
	}
	return nil
}

// applyFilters returns the items passing the feed's filter block. It runs
// after the feed's source items are merged and deduplicated, so a merged
// search+notification item carries its author, labels, and reason.
func applyFilters(def FeedDef, items []liveItem) []liveItem {
	f := def.Filters
	if f.IsZero() {
		return items
	}
	out := make([]liveItem, 0, len(items))
	for _, li := range items {
		if f.matches(li.item) {
			out = append(out, li)
		}
	}
	return out
}

func (f FilterDef) matches(item Item) bool {
	if matchAnyGlob(f.ExcludeRepos, item.Repo) {
		return false
	}
	if len(f.Repos) > 0 && !matchAnyGlob(f.Repos, item.Repo) {
		return false
	}
	if matchAnyAuthorGlob(f.ExcludeAuthors, item.Author) {
		return false
	}
	if len(f.Authors) > 0 && !matchAnyAuthorGlob(f.Authors, item.Author) {
		return false
	}
	if matchAnyLabel(f.ExcludeLabels, item.Labels) {
		return false
	}
	if len(f.Labels) > 0 && !matchAnyLabel(f.Labels, item.Labels) {
		return false
	}
	if len(f.Types) > 0 && !containsFold(f.Types, item.Kind) {
		return false
	}
	// A missing reason (item.Reason == "") matches no reasons filter: a
	// reasons filter deliberately excludes search-only items.
	if len(f.Reasons) > 0 && !containsFold(f.Reasons, item.Reason) {
		return false
	}
	return true
}

func matchAnyGlob(patterns []string, value string) bool {
	for _, pattern := range patterns {
		// Match errors only on malformed patterns, which validation
		// rejected at load; a malformed pattern here just doesn't match.
		if ok, err := doublestar.Match(pattern, value); err == nil && ok {
			return true
		}
	}
	return false
}

// matchAnyAuthorGlob matches a login case-insensitively, with "[" and "]"
// escaped to literals so "*[bot]" excludes every bot login as written.
func matchAnyAuthorGlob(patterns []string, author string) bool {
	author = strings.ToLower(author)
	for _, pattern := range patterns {
		pattern = bracketEscaper.Replace(strings.ToLower(pattern))
		if ok, err := doublestar.Match(pattern, author); err == nil && ok {
			return true
		}
	}
	return false
}

var bracketEscaper = strings.NewReplacer("[", `\[`, "]", `\]`)

func matchAnyLabel(patterns, labels []string) bool {
	for _, label := range labels {
		if matchAnyGlob(patterns, label) {
			return true
		}
	}
	return false
}

func containsFold(values []string, value string) bool {
	for _, v := range values {
		if strings.EqualFold(v, value) {
			return true
		}
	}
	return false
}
