package agentws

import (
	"embed"
	"fmt"
)

// examplesFS holds the canonical worked example: an agent-workspace.yaml
// naming a shipped catalogue entry, an mcps.yaml entry and a skills.yml
// package, plus the two library files it references. internal/app/prompts
// embeds all three in the hive-agent-workspaces authoring prompt, following
// the actions.ExampleYAML() contract (internal/app/actions/docs.go) —
// TestExampleYAMLIsValid parses and validates these bytes, so a prompt example
// that no longer loads fails the build rather than misleading an agent.
//
//go:embed examples/agent-workspace.yaml examples/mcps.yaml examples/skills.yml
var examplesFS embed.FS

// ExampleWorkspaceYAML is the canonical agent-workspace.yaml document.
func ExampleWorkspaceYAML() string {
	return mustReadExample("agent-workspace.yaml")
}

// ExampleMCPsYAML is the canonical mcps.yaml document the example workspace
// references.
func ExampleMCPsYAML() string {
	return mustReadExample("mcps.yaml")
}

// ExampleSkillsYAML is the canonical skills.yml document: the hive package the
// example workspace enables, plus one authored package showing exclude.
func ExampleSkillsYAML() string {
	return mustReadExample("skills.yml")
}

func mustReadExample(name string) string {
	data, err := examplesFS.ReadFile("examples/" + name)
	if err != nil {
		// Unreachable: the file is embedded at compile time.
		panic(fmt.Sprintf("agentws: reading embedded example %q: %v", name, err))
	}
	return string(data)
}
