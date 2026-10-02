package flow

import (
	"fmt"
	"path/filepath"
	"strings"
)

// LaunchSessionConfig is a launch-session node: 1 input, 0 outputs
// (terminal). Every item that reaches it starts a repository-backed hive
// session, with the same template data an actions.yml launch-session action
// renders over. It carries the launch inline so a flow that is the only
// caller does not need a catalog entry.
type LaunchSessionConfig struct {
	// Repo renders the repository the session is created against.
	Repo string `json:"repo" yaml:"repo"`
	// Agent selects a non-default hive agent profile. Empty means the
	// launcher's default.
	Agent string `json:"agent,omitempty" yaml:"agent,omitempty"`
	// SessionName renders the session's name. Empty derives one from the node
	// and the item key. It is not `name`, which is the node's own label.
	SessionName string `json:"sessionName,omitempty" yaml:"sessionName,omitempty"`
	// Prompt renders the session's initial prompt.
	Prompt string `json:"prompt" yaml:"prompt"`
}

func (c *LaunchSessionConfig) Inputs() int  { return 1 }
func (c *LaunchSessionConfig) Outputs() int { return 0 }

func (c *LaunchSessionConfig) Validate(Refs) error {
	if strings.TrimSpace(c.Repo) == "" {
		return fmt.Errorf("repo: repo is required")
	}
	if strings.TrimSpace(c.Prompt) == "" {
		return fmt.Errorf("prompt: prompt is required")
	}
	return nil
}

// LaunchChatConfig is a launch-chat node: 1 input, 0 outputs (terminal).
// Every item that reaches it opens a chat in an agent workspace, with the
// rendered prompt as the chat's first message.
type LaunchChatConfig struct {
	// Workspace is the agent workspace's directory name under the configured
	// workspace root.
	Workspace string `json:"workspace" yaml:"workspace"`
	// Prompt renders the chat's opening message.
	Prompt string `json:"prompt" yaml:"prompt"`
}

func (c *LaunchChatConfig) Inputs() int  { return 1 }
func (c *LaunchChatConfig) Outputs() int { return 0 }

func (c *LaunchChatConfig) Validate(Refs) error {
	workspace := strings.TrimSpace(c.Workspace)
	if workspace == "" {
		return fmt.Errorf("workspace: workspace is required")
	}
	if workspace != c.Workspace || workspace == "." || filepath.Base(workspace) != workspace || !filepath.IsLocal(workspace) {
		return fmt.Errorf("workspace: %q is not a workspace directory name", c.Workspace)
	}
	if strings.TrimSpace(c.Prompt) == "" {
		return fmt.Errorf("prompt: prompt is required")
	}
	return nil
}
