package flow

import "fmt"

// FeedConfig is a feed node: 1 input, 0 outputs (terminal). The node *is* the
// feed — its identity is the node id (flow-qualified as "<flowId>/<nodeId>"
// when persisting feed_item rows), so there is no config to carry.
type FeedConfig struct{}

func (c *FeedConfig) Inputs() int  { return 1 }
func (c *FeedConfig) Outputs() int { return 0 }

func (c *FeedConfig) Validate(Refs) error { return nil }

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
	return nil
}
