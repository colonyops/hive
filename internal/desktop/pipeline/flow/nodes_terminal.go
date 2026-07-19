package flow

import "fmt"

// FeedConfig is a feed node: 1 input, 0 outputs (terminal). Its results are
// relayed to Go and persisted against the referenced profiles feed id.
type FeedConfig struct {
	Feed string `json:"feed" yaml:"feed"`
}

func (c *FeedConfig) Inputs() int  { return 1 }
func (c *FeedConfig) Outputs() int { return 0 }

func (c *FeedConfig) Validate(refs Refs) error {
	if c.Feed == "" {
		return fmt.Errorf("feed: feed is required")
	}
	if !validSlug(c.Feed) {
		return fmt.Errorf("feed: feed %q is not a valid id", c.Feed)
	}
	if !refsResolveFeed(refs, c.Feed) {
		return fmt.Errorf("feed: feed %q: unresolved reference", c.Feed)
	}
	return nil
}

// ActionConfig is an action node: 1 input, 0 outputs (terminal). It enqueues
// an output_command against the referenced .hive/actions.yml action id.
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
	return nil
}
