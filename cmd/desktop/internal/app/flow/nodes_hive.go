package flow

import (
	"fmt"
	"strings"

	"github.com/colonyops/hive/pkg/tmpl"
)

// PublishMessageConfig is a terminal node that publishes one message per
// item update to a hive messaging topic, like a publish-message action.
type PublishMessageConfig struct {
	// Topic is literal: a wildcard or template would let one node fan out to
	// topics nobody configured.
	Topic   string `json:"topic"   yaml:"topic"`
	Message string `json:"message" yaml:"message"`
}

func (c *PublishMessageConfig) Inputs() int  { return 1 }
func (c *PublishMessageConfig) Outputs() int { return 0 }

func (c *PublishMessageConfig) Validate(Refs) error {
	topic := strings.TrimSpace(c.Topic)
	if topic == "" {
		return fmt.Errorf("topic: topic is required")
	}
	if topic != c.Topic || strings.ContainsAny(topic, "* \t\n") || strings.Contains(topic, "{{") {
		return fmt.Errorf("topic: %q must be a literal topic without spaces, wildcards, or templates", c.Topic)
	}
	if strings.TrimSpace(c.Message) == "" {
		return fmt.Errorf("message: message is required")
	}
	return validateTemplate("message", c.Message)
}

// CreateTaskConfig is a terminal node that creates one hive task per item.
type CreateTaskConfig struct {
	Title       string `json:"title"                 yaml:"title"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	// Repo renders the task's repo key ("owner/repo"). Empty files the task
	// under no repository.
	Repo string `json:"repo,omitempty" yaml:"repo,omitempty"`
	// Parent is the literal id of an existing epic or task.
	Parent string `json:"parent,omitempty" yaml:"parent,omitempty"`
}

func (c *CreateTaskConfig) Inputs() int  { return 1 }
func (c *CreateTaskConfig) Outputs() int { return 0 }

func (c *CreateTaskConfig) Validate(Refs) error {
	if strings.TrimSpace(c.Title) == "" {
		return fmt.Errorf("title: title is required")
	}
	if err := validateTemplate("title", c.Title); err != nil {
		return err
	}
	if err := validateTemplate("description", c.Description); err != nil {
		return err
	}
	if err := validateTemplate("repo", c.Repo); err != nil {
		return err
	}
	if c.Parent != strings.TrimSpace(c.Parent) || strings.ContainsAny(c.Parent, " \t\n") || strings.Contains(c.Parent, "{{") {
		return fmt.Errorf("parent: %q is not a task id", c.Parent)
	}
	return nil
}

func validateTemplate(field, text string) error {
	if err := tmpl.NewValidation().ValidateSyntax(text); err != nil {
		return fmt.Errorf("%s: %w", field, err)
	}
	return nil
}
