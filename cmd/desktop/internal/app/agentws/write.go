package agentws

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/colonyops/hive/cmd/desktop/internal/app/schedule"
	configmigrate "github.com/colonyops/hive/internal/config/migrate"
	"github.com/colonyops/hive/pkg/atomicfile"
)

// This file is the manifest writer the Migration Notes reserved: it edits the
// parsed yaml.Node tree in place (the flow/yamldoc.go pattern) so a
// hand-authored agent-workspace.yaml keeps its comments, key order, and any
// keys this build does not know. A yaml.Marshal round-trip would destroy all
// three, which is why Workspace itself stays read-only. The node helpers are
// deliberately package-local, the same self-containment flow and actions keep.

// agentsScaffold is the AGENTS.md a created workspace starts with. It is
// written exactly once, at creation, and is authored from then on (ADR workspace-directories-are-generated-and-disposable
// §3, ADR a-created-workspace-starts-with-an-agents-md-scaffold): the user or the agent overrides it by editing the file, and
// nothing ever regenerates it — the same rule the seeded hive workspace's
// AGENTS.md follows.
const agentsScaffold = `# %[1]s

This is the %[1]q agent workspace — a user-defined directory Hive Desktop
pre-configures with the MCP servers and skills chosen for its purpose. Prefer
the tools this workspace declares over ad-hoc alternatives, and keep anything
worth keeping under this directory; docs/ persists between sessions.

Hive copies this file to CLAUDE.md when the workspace opens and never edits
it. Replace any of it — including this paragraph — with the instructions this
workspace's agent should follow.

## Purpose

Describe what this workspace is for: the tasks its agent is expected to
handle, and how it should approach them.
`

// ManifestEdit is the set of manifest fields a writer owns: what the in-app
// editor sets, and nothing else. Everything the file says beyond these keys —
// comments, key order, keys this build does not know — survives a write.
type ManifestEdit struct {
	Name    string
	Command string
	MCPs    []string
	Skills  []string
	// Schedules is the whole schedules: list, not a delta: the editor holds
	// every entry while it is open, so a write reconciles the file to exactly
	// this set.
	Schedules []schedule.Spec
}

// CreateWorkspace makes dir under root, writes its first manifest, and seeds
// an AGENTS.md scaffold for the user to shape. The directory already existing
// is returned as-is so the caller can classify it (errors.Is(err, fs.ErrExist)).
func CreateWorkspace(root, dir string, edit ManifestEdit) error {
	if err := os.Mkdir(filepath.Join(root, dir), 0o700); err != nil {
		return err
	}
	if err := WriteManifest(root, dir, edit); err != nil {
		return err
	}
	scaffold := fmt.Sprintf(agentsScaffold, edit.Name)
	if err := os.WriteFile(filepath.Join(root, dir, "AGENTS.md"), []byte(scaffold), 0o600); err != nil {
		return fmt.Errorf("AGENTS.md: write: %w", err)
	}
	return nil
}

// RemoveWorkspace deletes dir and everything under it: the manifest, the
// generated trees, and the authored files the user and the agent wrote there,
// canvases included (ADR deleting-a-workspace-deletes-its-directory).
//
// It re-checks the target itself rather than trusting the caller, because it
// is the one call in this package that removes files it did not write: dir
// must name a single directory under root, and that directory must hold a
// manifest. A root with no workspace in it, .shared, and a symlink pointing
// somewhere else all fail those checks, so the worst a wrong dir can do is
// nothing.
func RemoveWorkspace(root, dir string) error {
	if dir == "" || filepath.Base(dir) != dir || !filepath.IsLocal(dir) {
		return fmt.Errorf("agentws: %q is not a workspace directory name", dir)
	}
	path := filepath.Join(root, dir)
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("agentws: workspace %s: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("agentws: workspace %s is not a directory", path)
	}
	if _, err := os.Stat(filepath.Join(path, manifestFileName)); err != nil {
		return fmt.Errorf("agentws: workspace %s: %w", path, err)
	}
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("agentws: delete workspace %s: %w", path, err)
	}
	return nil
}

// WriteManifest sets exactly the ManifestEdit fields in dir's
// agent-workspace.yaml, creating a fresh version-current document when the
// file does not exist and editing the existing document in place when it
// does — everything else the file says survives. An empty mcps, skills or
// schedules removes the key rather than writing an empty list.
func WriteManifest(root, dir string, edit ManifestEdit) error {
	path := filepath.Join(root, dir, manifestFileName)

	var doc, mapping *yaml.Node
	raw, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err):
		mapping = &yaml.Node{Kind: yaml.MappingNode}
		doc = &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{mapping}}
		if err := setManifestValue(mapping, "version", configmigrate.AgentWorkspaceSet.Current); err != nil {
			return err
		}
	case err != nil:
		return fmt.Errorf("agent-workspace.yaml: %w", err)
	default:
		doc, mapping, err = parseManifestNode(raw)
		if err != nil {
			return err
		}
	}

	for _, field := range []struct {
		key   string
		value any
	}{
		{"name", edit.Name},
		{"command", edit.Command},
	} {
		if err := setManifestValue(mapping, field.key, field.value); err != nil {
			return err
		}
	}
	for _, list := range []struct {
		key    string
		values []string
	}{
		{"mcps", edit.MCPs},
		{"skills", edit.Skills},
	} {
		if len(list.values) == 0 {
			removeManifestKey(mapping, list.key)
			continue
		}
		if err := setManifestValue(mapping, list.key, list.values); err != nil {
			return err
		}
	}
	reconcileSchedules(mapping, edit.Schedules)

	out, err := encodeManifestDoc(doc)
	if err != nil {
		return err
	}
	if err := writeFileAtomic(path, out); err != nil {
		return fmt.Errorf("agent-workspace.yaml: %w", err)
	}
	return nil
}

func parseManifestNode(data []byte) (doc, mapping *yaml.Node, err error) {
	doc = &yaml.Node{}
	if err := yaml.Unmarshal(data, doc); err != nil {
		return nil, nil, fmt.Errorf("agent-workspace.yaml: parse: %w", err)
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, nil, fmt.Errorf("agent-workspace.yaml: document is not a mapping")
	}
	return doc, doc.Content[0], nil
}

// setManifestValue encodes value and sets it at mapping[key], appending the
// pair when key is absent. Replacing only the value node is what keeps the
// key's own head comment attached.
func setManifestValue(mapping *yaml.Node, key string, value any) error {
	var v yaml.Node
	if err := v.Encode(value); err != nil {
		return fmt.Errorf("agent-workspace.yaml: encode %s: %w", key, err)
	}
	setManifestNode(mapping, key, &v)
	return nil
}

// setManifestNode is setManifestValue over an already-built node, for values
// whose YAML style matters (a block scalar, say).
func setManifestNode(mapping *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content[i+1] = value
			return
		}
	}
	mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, value)
}

// removeManifestKey drops key and its value from mapping. The key's own head
// comment goes with it — a comment on a key that no longer exists would
// otherwise reattach to whatever follows.
func removeManifestKey(mapping *yaml.Node, key string) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
			return
		}
	}
}

func encodeManifestDoc(doc *yaml.Node) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, fmt.Errorf("agent-workspace.yaml: encode: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("agent-workspace.yaml: encode: %w", err)
	}
	return buf.Bytes(), nil
}

func writeFileAtomic(path string, data []byte) error {
	if err := atomicfile.Write(path, data, 0o600); err != nil {
		return fmt.Errorf("replace: %w", err)
	}
	return nil
}
