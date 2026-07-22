package flow

import (
	"fmt"
	"unicode/utf8"
)

// feedIcons is the scoped set of sidebar glyphs a feed node may carry. It is
// intentionally small (a curated list rather than every available icon) and
// must stay in sync with the frontend's feed icon registry
// (desktop/frontend/src/lib/feedIcons.ts). An empty Icon means "use the
// sidebar default", so the empty string is always allowed.
var feedIcons = map[string]bool{
	"git-branch":       true,
	"git-pull-request": true,
	"circle-dot":       true,
	"message-square":   true,
	"at-sign":          true,
	"rss":              true,
	"bell":             true,
	"eye":              true,
	"star":             true,
	"bug":              true,
	"shield":           true,
	"zap":              true,
	"sparkles":         true,
	"flag":             true,
	"inbox":            true,
	"users":            true,
	"tag":              true,
	"package":          true,
	"rocket":           true,
	"clock":            true,
}

// feedDescriptionMaxLen caps a feed's hover description. It is generous enough
// for a sentence or two of context (useful for LLM-generated feeds) while
// keeping the persisted YAML and the sidebar tooltip bounded.
const feedDescriptionMaxLen = 500

// FeedConfig is a feed node: 1 input, 0 outputs (terminal). The node *is* the
// feed — its identity is the node id (flow-qualified as "<flowId>/<nodeId>"
// for membership claims). Icon and Description are purely cosmetic
// sidebar presentation: the icon shown in the tree and the tooltip surfaced
// on hover (handy context for LLM-generated feeds). Both are optional.
type FeedConfig struct {
	Icon        string `json:"icon,omitempty"        yaml:"icon,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

func (c *FeedConfig) Inputs() int  { return 1 }
func (c *FeedConfig) Outputs() int { return 0 }

func (c *FeedConfig) Validate(Refs) error {
	if c.Icon != "" && !feedIcons[c.Icon] {
		return fmt.Errorf("icon: %q is not a supported feed icon", c.Icon)
	}
	if utf8.RuneCountInString(c.Description) > feedDescriptionMaxLen {
		return fmt.Errorf("description: must be at most %d characters", feedDescriptionMaxLen)
	}
	return nil
}

// ActionConfig is an action node: 1 input, 0 outputs (terminal). It enqueues
// an output_command against the referenced desktop actions.yml action id.
type ActionConfig struct {
	Action string `json:"action" yaml:"action"`
}

func (c *ActionConfig) Inputs() int  { return 1 }
func (c *ActionConfig) Outputs() int { return 0 }

func (c *ActionConfig) Validate(refs Refs) error {
	if c.Action == "" {
		return fmt.Errorf("action: action is required")
	}
	if !validSlug(c.Action) {
		return fmt.Errorf("action: action %q is not a valid id", c.Action)
	}
	if !refsResolveAction(refs, c.Action) {
		return fmt.Errorf("action: action %q: unresolved reference", c.Action)
	}
	if !refsActionHeadlessCapable(refs, c.Action) {
		return fmt.Errorf("action: action %q requires interactive session input and cannot run in a flow", c.Action)
	}
	return nil
}
