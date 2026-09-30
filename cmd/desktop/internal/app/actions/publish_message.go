package actions

import (
	"fmt"
	"strings"
)

// PublishMessageConfig publishes a rendered, durable message to one literal
// topic. Topics are intentionally not templates or wildcards: action authors
// choose the routing at configuration time.
type PublishMessageConfig struct {
	MessageTemplate string `yaml:"message_template"`
	Topic           string `yaml:"topic"`
}

func (c *PublishMessageConfig) Validate() error {
	if strings.TrimSpace(c.MessageTemplate) == "" {
		return fmt.Errorf("publish-message: message_template is required")
	}
	topic := strings.TrimSpace(c.Topic)
	if topic == "" {
		return fmt.Errorf("publish-message: topic is required")
	}
	if strings.ContainsAny(topic, "*") || strings.Contains(topic, "{{") || strings.Contains(topic, "}}") {
		return fmt.Errorf("publish-message: topic must be a constant literal without wildcards or templates")
	}
	return nil
}
