package agentws

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/colonyops/hive/cmd/desktop/internal/app/schedule"
)

const schedulesKey = "schedules"

// WriteSchedules makes the manifest's schedules: list say exactly specs and
// touches nothing else in the file.
func WriteSchedules(root, dir string, specs []schedule.Spec) error {
	path := filepath.Join(root, dir, manifestFileName)
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("agent-workspace.yaml: %w", err)
	}
	doc, mapping, err := parseManifestNode(raw)
	if err != nil {
		return err
	}
	reconcileSchedules(mapping, specs)
	out, err := encodeManifestDoc(doc)
	if err != nil {
		return err
	}
	if err := writeFileAtomic(path, out); err != nil {
		return fmt.Errorf("agent-workspace.yaml: %w", err)
	}
	return nil
}

// reconcileSchedules is a node-tree edit for the same reason the rest of
// WriteManifest is: keys and comments a Spec does not own survive.
func reconcileSchedules(mapping *yaml.Node, specs []schedule.Spec) {
	if len(specs) == 0 {
		removeManifestKey(mapping, schedulesKey)
		return
	}
	sequence := manifestSequence(mapping, schedulesKey)
	kept := make([]*yaml.Node, 0, len(specs))
	for _, spec := range specs {
		entry := scheduleEntry(sequence, spec.ID)
		if entry == nil {
			entry = &yaml.Node{Kind: yaml.MappingNode}
		}
		applySchedule(entry, spec)
		kept = append(kept, entry)
	}
	sequence.Content = kept
}

// applySchedule writes the keys a Spec owns onto one sequence entry. An empty
// name, disabled and on_missed are removed at their default rather than
// written, so a file nobody has customized stays as short as the one the docs
// show. The name especially: writing DisplayName() would persist the id as a
// name the user never typed, and the entry could never go back to having none.
func applySchedule(entry *yaml.Node, spec schedule.Spec) {
	setManifestNode(entry, "id", scalar(spec.ID))
	if spec.Name != "" {
		setManifestNode(entry, "name", scalar(spec.Name))
	} else {
		removeManifestKey(entry, "name")
	}
	setManifestNode(entry, "cron", scalar(spec.Cron))
	setManifestNode(entry, "prompt", promptScalar(spec.Prompt))

	if spec.Disabled {
		setManifestNode(entry, "disabled", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"})
	} else {
		removeManifestKey(entry, "disabled")
	}

	if spec.OnMissed != "" && spec.OnMissed != schedule.OnMissedRun {
		setManifestNode(entry, "on_missed", scalar(string(spec.OnMissed)))
	} else {
		removeManifestKey(entry, "on_missed")
	}
}

func scalar(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

// promptScalar renders a multi-line prompt as a literal block so the file
// stays readable: a prompt is prose the user edits by hand, and the folded or
// quoted forms turn it into one unreadable line.
func promptScalar(prompt string) *yaml.Node {
	node := scalar(prompt)
	if strings.Contains(prompt, "\n") {
		node.Style = yaml.LiteralStyle
	}
	return node
}

// A non-sequence value is replaced: `schedules:` with nothing under it parses
// as null, not as an empty list.
func manifestSequence(mapping *yaml.Node, key string) *yaml.Node {
	if node := findManifestValue(mapping, key); node != nil {
		if node.Kind != yaml.SequenceNode {
			*node = yaml.Node{Kind: yaml.SequenceNode}
		}
		return node
	}
	sequence := &yaml.Node{Kind: yaml.SequenceNode}
	mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, sequence)
	return sequence
}

func findManifestValue(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

func scheduleEntry(sequence *yaml.Node, id string) *yaml.Node {
	for _, entry := range sequence.Content {
		if scheduleID(entry) == id {
			return entry
		}
	}
	return nil
}

func scheduleID(entry *yaml.Node) string {
	if entry.Kind != yaml.MappingNode {
		return ""
	}
	if node := findManifestValue(entry, "id"); node != nil {
		return node.Value
	}
	return ""
}
