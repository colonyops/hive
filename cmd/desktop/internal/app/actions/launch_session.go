package actions

import (
	"fmt"
	"strings"
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
	// PostHook is a shell command rendered over the same data as the templates
	// above plus `.Session`, then run in the new checkout.
	PostHook string `yaml:"post_hook,omitempty"`
	// PostHookTimeout bounds the hook; zero means the executor's default.
	PostHookTimeout Duration `yaml:"post_hook_timeout,omitempty"`
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
	if c.PostHookTimeout < 0 {
		return fmt.Errorf("launch-session: post_hook_timeout must be positive")
	}
	if strings.TrimSpace(c.PostHook) == "" && c.PostHookTimeout != 0 {
		return fmt.Errorf("launch-session: post_hook_timeout is set without post_hook")
	}
	return nil
}
