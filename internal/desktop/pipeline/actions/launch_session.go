package actions

import (
	"fmt"
	"strings"
)

// LaunchSessionConfig is a launch-session action: it spawns a hive session
// from a triggering msg. PromptTemplate/RepoTemplate are Go text/template
// strings rendered over the msg payload by the output worker (see
// internal/desktop/pipeline's LaunchSessionExecutor) — this package only
// parses and validates the config, it never renders or executes it.
type LaunchSessionConfig struct {
	// PromptTemplate renders the new session's initial prompt.
	PromptTemplate string `yaml:"prompt_template"`
	// Agent optionally selects a non-default agent profile for the new
	// session (e.g. "claude", "aider").
	Agent string `yaml:"agent,omitempty"`
	// RepoTemplate optionally renders which repo the session is created
	// against; empty means the launcher's own default.
	RepoTemplate string `yaml:"repo_template,omitempty"`
	// Post is reserved for the design mock's "post-create mode" (board 5c) —
	// what happens right after the session spawns. Its semantics depend on
	// real session-spawn wiring, deferred past this phase; it is captured
	// here for schema completeness and passed through to the executor
	// unrendered, but the logging stub does nothing with it beyond logging
	// it.
	Post string `yaml:"post,omitempty"`
}

func (c *LaunchSessionConfig) Validate() error {
	if strings.TrimSpace(c.PromptTemplate) == "" {
		return fmt.Errorf("launch-session: prompt_template is required")
	}
	return nil
}
