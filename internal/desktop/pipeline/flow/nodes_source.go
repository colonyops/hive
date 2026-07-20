package flow

import (
	"fmt"
	"strings"
)

// Source fetch limits — kept local to the flow package (the package doc
// forbids a flow -> feed import). They mirror the GitHub API page caps the
// producer's fetch path enforces: search pages at 100, notifications at 50.
const (
	defaultSourceLimit    = 50
	maxSearchLimit        = 100
	maxNotificationsLimit = 50
)

// GithubSourceConfig is a github-source node: 0 inputs, 1 output. It embeds
// the GitHub fetch config directly (there is no separate profiles sources:
// list any more — a source IS this node), matching the desktop feed fetch
// model: a "search" source runs a query, a "notifications" source drains the
// authenticated user's inbox. The backend producer polls one such node per
// tick and appends its items to the event log under a flow-qualified topic.
type GithubSourceConfig struct {
	Kind  string `json:"kind"            yaml:"kind"`
	Query string `json:"query,omitempty" yaml:"query,omitempty"`
	Limit int    `json:"limit,omitempty" yaml:"limit,omitempty"`
}

func (c *GithubSourceConfig) Inputs() int  { return 0 }
func (c *GithubSourceConfig) Outputs() int { return 1 }

// Validate is Refs-free: a source node's config is self-contained. The rules
// mirror the desktop feed source validation (search needs a query and caps at
// 100; notifications forbids a query and caps at 50).
func (c *GithubSourceConfig) Validate(Refs) error {
	switch c.Kind {
	case "search":
		if strings.TrimSpace(c.Query) == "" {
			return fmt.Errorf("github-source: kind \"search\" requires a query")
		}
		if c.Limit > maxSearchLimit {
			return fmt.Errorf("github-source: limit %d exceeds the search API page cap of %d", c.Limit, maxSearchLimit)
		}
	case "notifications":
		if strings.TrimSpace(c.Query) != "" {
			return fmt.Errorf("github-source: kind \"notifications\" takes no query")
		}
		if c.Limit > maxNotificationsLimit {
			return fmt.Errorf("github-source: limit %d exceeds the notifications API page cap of %d", c.Limit, maxNotificationsLimit)
		}
	case "":
		return fmt.Errorf("github-source: kind is required (want \"search\" or \"notifications\")")
	default:
		return fmt.Errorf("github-source: unknown kind %q (want \"search\" or \"notifications\")", c.Kind)
	}
	if c.Limit < 0 {
		return fmt.Errorf("github-source: limit must not be negative")
	}
	return nil
}

// EffectiveLimit resolves the configured limit against the per-kind default,
// so the producer never has to repeat the fallback.
func (c *GithubSourceConfig) EffectiveLimit() int {
	if c.Limit > 0 {
		return c.Limit
	}
	return defaultSourceLimit
}
