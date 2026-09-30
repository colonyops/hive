package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
)

// filterableItem is the GitHub item shape a github-filter node inspects, read
// off a message's Payload. It mirrors the json tags of
// internal/app/sources/github/feed.Item — the same shape github_source.go
// encodes as a message payload. Fields it does not name are ignored.
type filterableItem struct {
	Repo   string   `json:"repo"`
	Author string   `json:"author"`
	Labels []string `json:"labels"`
	Kind   string   `json:"kind"`
	Reason string   `json:"reason"`
	CI     string   `json:"ci"`
	Review string   `json:"review"`
}

// filterProcessor evaluates one github-filter node. Its globs are compiled
// once when the Runner is built, so a filter costs one regexp match per
// pattern per message rather than a compile.
type filterProcessor struct {
	repos          []*regexp.Regexp
	excludeRepos   []*regexp.Regexp
	authors        []*regexp.Regexp
	excludeAuthors []*regexp.Regexp
	labels         []*regexp.Regexp
	excludeLabels  []*regexp.Regexp
	types          []string
	reasons        []string
	ci             []string
	review         []string
}

// newFilterNode builds a github-filter node's processor. Its globs compile
// once here, not once per message.
func newFilterNode(_ *Runner, _ string, config flow.NodeConfig) (processor, error) {
	cfg, ok := config.(*flow.GithubFilterConfig)
	if !ok {
		return nil, fmt.Errorf("github-filter: unexpected config type %T", config)
	}
	return &filterProcessor{
		repos:        compileGlobs(cfg.Repos, false),
		excludeRepos: compileGlobs(cfg.ExcludeRepos, false),
		// Authors match case-insensitively: both the pattern and the value are
		// lowered before matching.
		authors:        compileGlobs(cfg.Authors, true),
		excludeAuthors: compileGlobs(cfg.ExcludeAuthors, true),
		labels:         compileGlobs(cfg.Labels, false),
		excludeLabels:  compileGlobs(cfg.ExcludeLabels, false),
		types:          lowerAll(cfg.Types),
		reasons:        lowerAll(cfg.Reasons),
		ci:             lowerAll(cfg.CI),
		review:         lowerAll(cfg.Review),
	}, nil
}

// A filter is pure: it has nothing to reset and nothing to release.
func (p *filterProcessor) reset() {}
func (p *filterProcessor) close() {}

// process routes a message to port 0 (pass) or port 1 (fail). Leaving port 1
// unwired reproduces a plain "drop on fail" filter through the engine's
// unwired-port-becomes-discard rule.
func (p *filterProcessor) process(_ context.Context, msg models.Msg, _ NodeKV, _ ConsoleSink) ([][]models.Msg, error) {
	var item filterableItem
	// A payload that is not an object simply has no fields to filter on; the
	// zero item then fails any include group, which is the same outcome the
	// browser reached by reading properties off a non-object.
	_ = json.Unmarshal(msg.Payload, &item)

	if p.matches(item) {
		return [][]models.Msg{{msg}, nil}, nil
	}
	return [][]models.Msg{nil, {msg}}, nil
}

// matches applies one rule: groups AND together, values within a group OR,
// and exclude groups win over includes. A missing reason matches no reasons
// filter, so a reasons filter deliberately excludes search-only items.
func (p *filterProcessor) matches(item filterableItem) bool {
	if matchAny(p.excludeRepos, item.Repo) {
		return false
	}
	if len(p.repos) > 0 && !matchAny(p.repos, item.Repo) {
		return false
	}

	author := strings.ToLower(item.Author)
	if matchAny(p.excludeAuthors, author) {
		return false
	}
	if len(p.authors) > 0 && !matchAny(p.authors, author) {
		return false
	}

	if matchAnyOf(p.excludeLabels, item.Labels) {
		return false
	}
	if len(p.labels) > 0 && !matchAnyOf(p.labels, item.Labels) {
		return false
	}

	if len(p.types) > 0 && !containsFold(p.types, item.Kind) {
		return false
	}
	if len(p.reasons) > 0 && !containsFold(p.reasons, item.Reason) {
		return false
	}
	if len(p.ci) > 0 && !containsFold(p.ci, item.CI) {
		return false
	}
	if len(p.review) > 0 && !containsFold(p.review, item.Review) {
		return false
	}
	return true
}

func matchAny(patterns []*regexp.Regexp, value string) bool {
	for _, pattern := range patterns {
		if pattern.MatchString(value) {
			return true
		}
	}
	return false
}

func matchAnyOf(patterns []*regexp.Regexp, values []string) bool {
	for _, value := range values {
		if matchAny(patterns, value) {
			return true
		}
	}
	return false
}

// containsFold reports whether values contains value case-insensitively.
// values is already lowered.
func containsFold(values []string, value string) bool {
	return slices.Contains(values, strings.ToLower(value))
}

func lowerAll(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = strings.ToLower(v)
	}
	return out
}

func compileGlobs(patterns []string, fold bool) []*regexp.Regexp {
	if len(patterns) == 0 {
		return nil
	}
	out := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		if fold {
			pattern = strings.ToLower(pattern)
		}
		out = append(out, globToRegexp(pattern))
	}
	return out
}

// globToRegexp compiles one filter glob. It supports exactly two wildcards:
// a single star matches any run of characters except "/" (one path segment),
// and a double star matches any run including "/" (zero or more segments).
//
// Every other character is literal, including "[", "]", "?", "{" and "}".
// That is deliberate and load-bearing: it is the behaviour the shipped
// frontend matcher has (see nodes/github-filter/config.ts), which in turn
// reproduced a Go filter that escaped those characters before handing the
// pattern to doublestar. Matching with doublestar directly here would turn a
// pattern like "*[bot]" from a literal suffix into a character class and
// silently change which items a deployed flow routes.
//
// GithubFilterConfig.Validate still uses doublestar.ValidatePattern: that is a
// save-time shape check on what a user typed, not the matching rule.
func globToRegexp(pattern string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString(`\A`)
	for i := 0; i < len(pattern); i++ {
		switch {
		case pattern[i] == '*' && i+1 < len(pattern) && pattern[i+1] == '*':
			// "." excludes "\n" in Go and in JavaScript alike, and a negated
			// class like "[^/]" includes it in both; neither wildcard needs a
			// flag to agree with the frontend matcher.
			b.WriteString(`.*`)
			i++
		case pattern[i] == '*':
			b.WriteString(`[^/]*`)
		default:
			b.WriteString(regexp.QuoteMeta(pattern[i : i+1]))
		}
	}
	b.WriteString(`\z`)
	// The source is built entirely from QuoteMeta output and fixed fragments,
	// so it cannot fail to compile.
	return regexp.MustCompile(b.String())
}
