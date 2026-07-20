package actions

import (
	"fmt"
	"strings"
)

// PublishEventConfig is a publish-event action: it publishes the msg
// payload to Topic on execution (see internal/desktop/pipeline's
// PublishEventExecutor) — this package only parses and validates the
// config, it never publishes anything itself.
type PublishEventConfig struct {
	// Topic is the event bus topic the msg payload is published to.
	Topic string `yaml:"topic"`
}

func (c *PublishEventConfig) Validate() error {
	if strings.TrimSpace(c.Topic) == "" {
		return fmt.Errorf("publish-event: topic is required")
	}
	return nil
}
