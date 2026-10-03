// Package agent is the catalog of coding agents that hive init and the
// desktop onboarding offer. A profile for an agent not listed here is still
// valid config; the catalog only drives what setup suggests.
package agent

import "slices"

type Agent struct {
	// Name is the profile key in config.yaml and the command on PATH.
	Name  string
	Label string
	// SkipPermissionFlags turn off the agent's own permission prompts. Empty
	// for an agent with no known flag. It is a slice because opencode needs
	// two arguments.
	SkipPermissionFlags []string
}

var known = []Agent{
	{Name: "claude", Label: "Claude Code", SkipPermissionFlags: []string{"--dangerously-skip-permissions"}},
	{Name: "opencode", Label: "OpenCode", SkipPermissionFlags: []string{"--agent", "free-permissions-runner"}},
	{Name: "codex", Label: "Codex", SkipPermissionFlags: []string{"--full-auto"}},
	{Name: "pi", Label: "Pi"},
	{Name: "amp", Label: "Amp"},
	{Name: "copilot", Label: "GitHub Copilot"},
	{Name: "cursor", Label: "Cursor"},
}

// Known returns the catalog in display order. The result is a copy the caller
// may modify.
func Known() []Agent {
	out := make([]Agent, len(known))
	for i, a := range known {
		a.SkipPermissionFlags = slices.Clone(a.SkipPermissionFlags)
		out[i] = a
	}
	return out
}

func Lookup(name string) (Agent, bool) {
	for _, a := range Known() {
		if a.Name == name {
			return a, true
		}
	}
	return Agent{}, false
}
