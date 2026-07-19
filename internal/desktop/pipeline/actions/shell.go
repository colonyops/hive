package actions

import (
	"fmt"
	"strings"
)

// ShellConfig is a shell action: it runs an author-trusted shell command
// rendered from the msg payload. CommandTemplate is a Go text/template
// string (rendered with the shared "shq" shell-quoting helper available —
// see internal/desktop/pipeline's ShellExecutor) — this package only parses
// and validates the config, it never renders or executes it.
type ShellConfig struct {
	// CommandTemplate renders the command line to run via `sh -c`.
	CommandTemplate string `yaml:"command_template"`
	// Cwd optionally sets the working directory; empty means the desktop
	// process's own cwd.
	Cwd string `yaml:"cwd,omitempty"`
	// Timeout bounds how long the command may run; zero means no deadline
	// beyond the invoking context's own.
	Timeout Duration `yaml:"timeout,omitempty"`
	// Env optionally adds/overrides environment variables for the command.
	Env map[string]string `yaml:"env,omitempty"`
}

func (c *ShellConfig) Validate() error {
	if strings.TrimSpace(c.CommandTemplate) == "" {
		return fmt.Errorf("shell: command_template is required")
	}
	return nil
}
