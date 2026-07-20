package flow

import (
	"fmt"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// GithubFilterConfig is a github-filter node: 1 input, 2 outputs (port 0 =
// pass, port 1 = fail). Its fields mirror the profiles feed FilterDef:
// groups AND together, values within a group OR, and exclude groups win
// over includes.
type GithubFilterConfig struct {
	Repos          []string `json:"repos,omitempty"           yaml:"repos,omitempty"`
	ExcludeRepos   []string `json:"exclude_repos,omitempty"   yaml:"exclude_repos,omitempty"`
	Authors        []string `json:"authors,omitempty"         yaml:"authors,omitempty"`
	ExcludeAuthors []string `json:"exclude_authors,omitempty" yaml:"exclude_authors,omitempty"`
	Labels         []string `json:"labels,omitempty"          yaml:"labels,omitempty"`
	ExcludeLabels  []string `json:"exclude_labels,omitempty"  yaml:"exclude_labels,omitempty"`
	Types          []string `json:"types,omitempty"           yaml:"types,omitempty"`
	Reasons        []string `json:"reasons,omitempty"         yaml:"reasons,omitempty"`
}

func (c *GithubFilterConfig) Inputs() int  { return 1 }
func (c *GithubFilterConfig) Outputs() int { return 2 }

// githubFilterValidTypes are the allowed values of the types filter.
var githubFilterValidTypes = map[string]bool{"pr": true, "issue": true}

// githubFilterValidReasons are the notification reasons GitHub delivers.
var githubFilterValidReasons = map[string]bool{
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

// empty reports whether every filter group is unset — an empty filter would
// pass (or fail) everything, which is always a config mistake.
func (c *GithubFilterConfig) empty() bool {
	return len(c.Repos) == 0 && len(c.ExcludeRepos) == 0 &&
		len(c.Authors) == 0 && len(c.ExcludeAuthors) == 0 &&
		len(c.Labels) == 0 && len(c.ExcludeLabels) == 0 &&
		len(c.Types) == 0 && len(c.Reasons) == 0
}

func (c *GithubFilterConfig) Validate(Refs) error {
	if c.empty() {
		return fmt.Errorf("github-filter: at least one of repos, exclude_repos, authors, exclude_authors, labels, exclude_labels, types, reasons must be set")
	}

	globGroups := []struct {
		name     string
		patterns []string
	}{
		{"repos", c.Repos},
		{"exclude_repos", c.ExcludeRepos},
		{"authors", c.Authors},
		{"exclude_authors", c.ExcludeAuthors},
		{"labels", c.Labels},
		{"exclude_labels", c.ExcludeLabels},
	}
	for _, group := range globGroups {
		for _, pattern := range group.patterns {
			if !doublestar.ValidatePattern(pattern) {
				return fmt.Errorf("github-filter: invalid %s glob %q", group.name, pattern)
			}
		}
	}
	for _, t := range c.Types {
		if !githubFilterValidTypes[strings.ToLower(t)] {
			return fmt.Errorf("github-filter: unknown type %q (want \"pr\" or \"issue\")", t)
		}
	}
	for _, reason := range c.Reasons {
		if !githubFilterValidReasons[reason] {
			return fmt.Errorf("github-filter: unknown reason %q", reason)
		}
	}
	return nil
}
