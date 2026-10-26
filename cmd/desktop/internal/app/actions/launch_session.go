package actions

import (
	"fmt"
	"strings"
	"time"

	"github.com/colonyops/hive/pkg/tmpl"
)

// LaunchSessionConfig starts either a repository-backed hive session or an
// agent workspace chat from a triggering message. This package only parses and
// validates the config; dispatch renders and executes it.
type LaunchSessionConfig struct {
	// PromptTemplate renders the new session's initial prompt.
	PromptTemplate string `yaml:"prompt_template"`
	// Agent optionally selects a non-default agent profile for the new
	// session (e.g. "claude", "aider").
	Agent string `yaml:"agent,omitempty"`
	// RepoTemplate selects a repository after rendering. Empty leaves target
	// selection to Workspace or invocation input.
	RepoTemplate string `yaml:"repo_template,omitempty"`
	// Workspace names an agent workspace by its directory under the configured
	// workspace root.
	Workspace string `yaml:"workspace,omitempty"`
	// Labels are templates, each rendered into one label on the new
	// session. A label that renders blank is dropped.
	Labels []string `yaml:"labels,omitempty"`
	// PostHook is a shell command rendered over the same data as the templates
	// above plus `.Session`, then run in the new checkout.
	PostHook string `yaml:"post_hook,omitempty"`
	// PostHookTimeout bounds the hook; zero means the executor's default.
	PostHookTimeout time.Duration `yaml:"post_hook_timeout,omitempty"`
}

func (c *LaunchSessionConfig) Validate() error {
	if strings.TrimSpace(c.PromptTemplate) == "" {
		return fmt.Errorf("launch-session: prompt_template is required")
	}
	repo := strings.TrimSpace(c.RepoTemplate)
	workspace := strings.TrimSpace(c.Workspace)
	if repo != "" && workspace != "" {
		return fmt.Errorf("launch-session: repo_template and workspace are mutually exclusive")
	}
	if workspace != "" && strings.TrimSpace(c.Agent) != "" {
		return fmt.Errorf("launch-session: agent cannot be set with workspace; the workspace command selects the agent")
	}
	if workspace != "" && strings.TrimSpace(c.PostHook) != "" {
		return fmt.Errorf("launch-session: post_hook cannot be set with workspace")
	}
	if workspace != "" && len(c.Labels) > 0 {
		return fmt.Errorf("launch-session: labels cannot be set with workspace; a chat is not a hive session")
	}
	if err := ValidateLabelTemplates(c.Labels); err != nil {
		return fmt.Errorf("launch-session: %w", err)
	}
	if c.PostHookTimeout < 0 {
		return fmt.Errorf("launch-session: post_hook_timeout must be positive")
	}
	if strings.TrimSpace(c.PostHook) == "" && c.PostHookTimeout != 0 {
		return fmt.Errorf("launch-session: post_hook_timeout is set without post_hook")
	}
	return nil
}

// ValidateLabelTemplates checks each label template's syntax. What a template
// renders to is checked at launch, when the data exists.
func ValidateLabelTemplates(labels []string) error {
	for i, label := range labels {
		if strings.TrimSpace(label) == "" {
			return fmt.Errorf("labels[%d] is blank", i)
		}
		if err := tmpl.NewValidation().ValidateSyntax(label); err != nil {
			return fmt.Errorf("labels[%d]: %w", i, err)
		}
	}
	return nil
}
