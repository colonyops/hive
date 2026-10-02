package flow

import (
	"fmt"
	"path/filepath"
	"strings"
)

// LaunchSessionConfig is a terminal node that starts a hive session per item,
// rendering the same templates as an actions.yml launch-session action.
type LaunchSessionConfig struct {
	Repo  string `json:"repo"            yaml:"repo"`
	Agent string `json:"agent,omitempty" yaml:"agent,omitempty"`
	// Not `name`: that key is the node's own label.
	SessionName string `json:"sessionName,omitempty" yaml:"sessionName,omitempty"`
	Prompt      string `json:"prompt"                yaml:"prompt"`
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

// LaunchChatConfig is a terminal node that opens an agent workspace chat per
// item, with the rendered prompt as the first message.
type LaunchChatConfig struct {
	// Workspace is a directory name under the workspace root.
	Workspace string `json:"workspace" yaml:"workspace"`
	Prompt    string `json:"prompt"    yaml:"prompt"`
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
