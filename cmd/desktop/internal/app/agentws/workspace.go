package agentws

import (
	"fmt"
	"slices"

	"github.com/colonyops/hive/cmd/desktop/internal/app/schedule"
)

// Workspace is one agent-workspace.yaml. Dir is the directory name under the
// root — the identity a session record stores, so a record survives a machine
// whose root is somewhere else.
//
// It is read-only: the manifest writer is write.go's node-tree editor
// (WriteManifest), never a yaml.Marshal of this struct, which would destroy
// comments, key order, and keys this build does not know.
type Workspace struct {
	Dir string `yaml:"-"`

	Version int    `yaml:"version"`
	Name    string `yaml:"name"`
	// Command is the launch template — the whole invocation, rendered against
	// LaunchData at spawn time. It replaced the ask/auto/full posture enum,
	// which could only express the two agents the launch table knew.
	Command string   `yaml:"command"`
	MCPs    []string `yaml:"mcps,omitempty"`
	// Skills names skill packages defined in skills.yml, not individual
	// skills — the unit a workspace enables is the package (ADR skill-packages-are-the-unit-a-workspace-enables).
	Skills    []string        `yaml:"skills,omitempty"`
	Schedules []schedule.Spec `yaml:"schedules,omitempty"`
}

// withDir stamps dir onto the workspace and onto every schedule it owns. A
// Spec's Workspace is not in the file: it travels to the scheduler on its own
// and has to carry the directory it came from.
func (w Workspace) withDir(dir string) Workspace {
	w.Dir = dir
	for i := range w.Schedules {
		w.Schedules[i].Workspace = dir
	}
	return w
}

// Agent is the label AgentFor reads off Command. There is no agent: key: a
// stored copy of a word already in the command goes wrong the moment someone
// edits the command by hand.
func (w Workspace) Agent() string {
	return AgentFor(w.Command)
}

// Validate checks the fields Workspace owns directly. Command is parsed as a
// template here rather than at launch: a manifest whose template is malformed
// is a broken workspace the list can explain, not a session that fails to
// start with nothing said until someone presses the button.
func (w Workspace) Validate() error {
	if w.Name == "" {
		return fmt.Errorf("agent-workspace.yaml: name is required")
	}
	if w.Command == "" {
		return fmt.Errorf("agent-workspace.yaml: command is required")
	}
	if err := ValidateCommand(w.Command); err != nil {
		return fmt.Errorf("agent-workspace.yaml: %w", err)
	}
	if dup := firstDuplicate(w.MCPs); dup != "" {
		return fmt.Errorf("agent-workspace.yaml: duplicate mcp %q", dup)
	}
	if dup := firstDuplicate(w.Skills); dup != "" {
		return fmt.Errorf("agent-workspace.yaml: duplicate skill package %q", dup)
	}
	if slices.Contains(w.MCPs, "") {
		return fmt.Errorf("agent-workspace.yaml: mcps entries must not be empty")
	}
	if slices.Contains(w.Skills, "") {
		return fmt.Errorf("agent-workspace.yaml: skill package entries must not be empty")
	}
	// A schedule whose prompt the command drops would start an agent that sits
	// idle in a detached session with nobody watching, so it is a workspace
	// problem the list can explain rather than a run that silently does
	// nothing.
	if len(w.Schedules) > 0 && !SupportsPrompt(w.Command) {
		return fmt.Errorf("agent-workspace.yaml: schedules need a command that passes the prompt; add%s", PromptTail)
	}
	seen := make(map[string]bool, len(w.Schedules))
	for _, spec := range w.Schedules {
		if err := spec.Validate(); err != nil {
			return fmt.Errorf("agent-workspace.yaml: %w", err)
		}
		if seen[spec.ID] {
			return fmt.Errorf("agent-workspace.yaml: duplicate schedule %q", spec.ID)
		}
		seen[spec.ID] = true
	}
	return nil
}

// firstDuplicate returns the first value that repeats in ids, or "" when
// every value is unique.
func firstDuplicate(ids []string) string {
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			return id
		}
		seen[id] = true
	}
	return ""
}
