package hiveconf

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Edit is the set of Hive config fields the desktop owns: the workspaces the
// session launcher scans, the agent profiles it offers, and which of them is
// the default. Comments, key order and keys this build does not know survive a
// write as parsed; blank lines and indentation are re-emitted by yaml.v3.
//
// Profiles and Workspaces are each the whole set, not a delta: the editor
// holds every entry while it is open, so a write reconciles the file to
// exactly what it was given (the agentws/write.go pattern).
type Edit struct {
	DefaultAgent string
	Profiles     []Profile
	Workspaces   []string
}

// reservedAgentKeys are the keys inside hive's `agents:` mapping that are not
// a profile. Reconciling profiles must step over them, or a write would delete
// the user's agent_selector setting — or, for "<<", a merge key the user
// wrote. Hive does not resolve a merge key inside agents (it reads "<<" as a
// profile named "<<"), but the line is theirs, not this app's to remove.
var reservedAgentKeys = map[string]bool{"default": true, "agent_selector": true, "<<": true}

// InvalidEditError is an edit the user can fix, as opposed to a disk that
// would not take the write. It is a distinct type so a caller classifies it
// with errors.As rather than by reading the message.
type InvalidEditError struct{ Reason string }

func (e InvalidEditError) Error() string { return e.Reason }

func invalid(format string, args ...any) error {
	return InvalidEditError{Reason: fmt.Sprintf(format, args...)}
}

// Check inspects a candidate config before it replaces the file. Apply hands
// it the temp file's path and an error stops the rename. It exists because the
// loader whose opinion matters, hive's own, lives across the hivecore seam
// this package does not cross.
type Check func(path string) error

// normalized is the edit Apply validates and writes. Trimming here rather than
// in Validate keeps the two from drifting: hive's profile lookup is exact, so
// a default that validates trimmed but is written padded fails the next load.
func (e Edit) normalized() Edit {
	out := Edit{DefaultAgent: strings.TrimSpace(e.DefaultAgent)}
	for _, p := range e.Profiles {
		out.Profiles = append(out.Profiles, Profile{
			Name:    strings.TrimSpace(p.Name),
			Command: strings.TrimSpace(p.Command),
			Flags:   p.Flags,
		})
	}
	// Two spellings of one folder — ~/code and its expansion — would have hive
	// discover every repository under it twice.
	seen := make(map[string]bool, len(e.Workspaces))
	for _, w := range e.Workspaces {
		trimmed := strings.TrimSpace(w)
		key := filepath.Clean(ExpandTilde(trimmed))
		if trimmed == "" || seen[key] {
			continue
		}
		seen[key] = true
		out.Workspaces = append(out.Workspaces, trimmed)
	}
	return out
}

// Validate rejects edits that would make Hive reject the whole config. It runs
// before any file is touched.
func (e Edit) Validate() error {
	if len(e.Profiles) == 0 {
		return invalid("choose at least one agent")
	}
	seen := make(map[string]bool, len(e.Profiles))
	for _, p := range e.Profiles {
		if p.Name == "" {
			return invalid("an agent profile needs a name")
		}
		if reservedAgentKeys[p.Name] {
			return invalid("%q is a Hive setting, not an agent", p.Name)
		}
		if seen[p.Name] {
			return invalid("%q is listed twice", p.Name)
		}
		seen[p.Name] = true
	}
	if !seen[e.DefaultAgent] {
		return invalid("the default agent must be one of the agents you chose")
	}
	if len(e.Workspaces) == 0 {
		return invalid("add at least one folder that holds your repositories")
	}
	for _, w := range e.Workspaces {
		if err := ValidateWorkspace(w); err != nil {
			return invalid("%s: %s", w, err)
		}
	}
	return nil
}

// Apply reconciles the owned keys while preserving the rest of an existing
// document. A new or keyless file is rendered with explanatory comments.
// When provided, check runs against the candidate before replacement so Hive
// can reject an edit without breaking the next launch.
func Apply(path string, edit Edit, check Check) error {
	edit = edit.normalized()
	if err := edit.Validate(); err != nil {
		return err
	}
	if path == "" {
		return fmt.Errorf("no Hive config path is available")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create the Hive config directory: %w", err)
	}

	raw, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err):
		return writeFileAtomic(path, []byte(render(edit)), check)
	case err != nil:
		return fmt.Errorf("read %s: %w", filepath.Base(path), err)
	}
	// A dotfiles-managed config is a symlink. ReadFile followed it; the rename
	// below has to as well, or the link is replaced by a regular file and the
	// dotfiles repository keeps the old content.
	if target, err := filepath.EvalSymlinks(path); err == nil {
		path = target
	}

	doc := &yaml.Node{}
	if err := yaml.Unmarshal(raw, doc); err != nil {
		return fmt.Errorf("parse the Hive config: %w", err)
	}
	// yaml.v3 attaches a keyless document's comments to no node. Render it whole
	// so every newly filled file gets the same header.
	if len(doc.Content) == 0 {
		return writeFileAtomic(path, []byte(render(edit)), check)
	}

	out, err := editDocument(doc, edit)
	if err != nil {
		return err
	}
	return writeFileAtomic(path, out, check)
}

// Create writes the commented header and nothing else, for a user who asked to
// open the file in an editor before setup has anything to put in it. An
// existing file, symlink included, is left alone.
func Create(path string) error {
	if path == "" {
		return fmt.Errorf("no Hive config path is available")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create the Hive config directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644) //nolint:gosec // shared with the hive CLI, holds no secret
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("create %s: %w", filepath.Base(path), err)
	}
	if _, err := file.WriteString(header); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	return nil
}

func editDocument(doc *yaml.Node, edit Edit) ([]byte, error) {
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("the Hive config is not a YAML mapping")
	}
	if err := checkRules(root, edit); err != nil {
		return nil, err
	}

	var workspaces yaml.Node
	if err := workspaces.Encode(edit.Workspaces); err != nil {
		return nil, fmt.Errorf("encode workspaces: %w", err)
	}
	setNode(root, "workspaces", &workspaces)
	// A file still on hive's deprecated spelling would otherwise keep a
	// repo_dirs list that now disagrees with the workspaces beside it. Hive
	// reads repo_dirs only when workspaces is empty, so the stale key is not
	// merely redundant — it is what a downgrade would read.
	removeKey(root, "repo_dirs")

	agents := nodeFor(root, "agents")
	if agents == nil || agents.Kind != yaml.MappingNode {
		agents = &yaml.Node{Kind: yaml.MappingNode}
		setNode(root, "agents", agents)
	}
	if err := reconcileAgents(agents, edit); err != nil {
		return nil, err
	}

	return encodeDoc(doc)
}

// Hive validates rules[].agent against the profile map, and this package owns
// neither the rules nor a way to repair them.
func checkRules(root *yaml.Node, edit Edit) error {
	rules := nodeFor(root, "rules")
	if rules == nil || rules.Kind != yaml.SequenceNode {
		return nil
	}
	keep := make(map[string]bool, len(edit.Profiles))
	for _, p := range edit.Profiles {
		keep[p.Name] = true
	}
	for _, rule := range rules.Content {
		if rule.Kind != yaml.MappingNode {
			continue
		}
		agent := nodeFor(rule, "agent")
		if agent == nil || agent.Value == "" || keep[agent.Value] {
			continue
		}
		pattern := ""
		if p := nodeFor(rule, "pattern"); p != nil {
			pattern = p.Value
		}
		return invalid("a rule in your config (pattern %q) uses the agent %q; keep it selected or change the rule first", pattern, agent.Value)
	}
	return nil
}

// Preserve each surviving profile node so unknown keys stay where the user
// put them.
func reconcileAgents(agents *yaml.Node, edit Edit) error {
	if err := setScalar(agents, "default", edit.DefaultAgent); err != nil {
		return err
	}
	keep := make(map[string]bool, len(edit.Profiles))
	for _, p := range edit.Profiles {
		keep[p.Name] = true
	}
	for i := 0; i+1 < len(agents.Content); {
		key := agents.Content[i].Value
		if reservedAgentKeys[key] || keep[key] {
			i += 2
			continue
		}
		agents.Content = append(agents.Content[:i], agents.Content[i+2:]...)
	}
	for _, p := range edit.Profiles {
		profile := nodeFor(agents, p.Name)
		if profile == nil || profile.Kind != yaml.MappingNode {
			profile = &yaml.Node{Kind: yaml.MappingNode}
			setNode(agents, p.Name, profile)
		}
		command := p.Command
		if command == "" {
			command = p.Name
		}
		if err := setScalar(profile, "command", command); err != nil {
			return err
		}
		// An empty flags list is written rather than dropped: it is the
		// visible statement that this agent runs with no extra arguments,
		// which is what someone turning skip-permissions back off expects to
		// see in the file.
		flags := p.Flags
		if flags == nil {
			flags = []string{}
		}
		var node yaml.Node
		if err := node.Encode(flags); err != nil {
			return fmt.Errorf("encode %s flags: %w", p.Name, err)
		}
		node.Style = yaml.FlowStyle
		setNode(profile, "flags", &node)
	}
	return nil
}

const header = `# Hive configuration
#
# The ` + "`hive`" + ` CLI and Hive Desktop share this file. Hive Desktop owns the
# workspaces and agents keys: it writes them during first run and under
# Settings > Hive CLI. Both can also be edited here by hand.
#
# Everything else Hive supports — rules, tmux integration, keybindings, user
# commands — can be added to this file and is left alone by Hive Desktop.
# Reference: https://hive.colonyops.io/configuration
`

// Use a template rather than an encoded node tree because the comments are the
// point: this is often the only time the user sees what the file is for.
func render(edit Edit) string {
	var b strings.Builder
	b.WriteString(header)
	b.WriteString("\n# Parent folders that hold your git repositories. Point these at the\n")
	b.WriteString("# folder your repos sit in, for example ~/code — not at a repository.\n")
	b.WriteString("workspaces:\n")
	for _, w := range edit.Workspaces {
		fmt.Fprintf(&b, "  - %s\n", scalar(w))
	}
	b.WriteString("\n# The agents Hive can start a session with.\n")
	b.WriteString("agents:\n")
	fmt.Fprintf(&b, "  default: %s\n", scalar(edit.DefaultAgent))
	for _, p := range edit.Profiles {
		command := p.Command
		if command == "" {
			command = p.Name
		}
		fmt.Fprintf(&b, "  %s:\n", scalar(p.Name))
		fmt.Fprintf(&b, "    command: %s\n", scalar(command))
		fmt.Fprintf(&b, "    flags: %s\n", flagSequence(p.Flags))
	}
	return b.String()
}

// Manual template output must quote values whose YAML meaning would change.
func scalar(s string) string {
	out, err := yaml.Marshal(s)
	if err != nil {
		return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	}
	return strings.TrimRight(string(out), "\n")
}

func flagSequence(flags []string) string {
	if len(flags) == 0 {
		return "[]"
	}
	node := &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle}
	for _, f := range flags {
		node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: f})
	}
	out, err := yaml.Marshal(node)
	if err != nil {
		return "[]"
	}
	return strings.TrimRight(string(out), "\n")
}

// The node helpers below are the flow/yamldoc.go pattern, kept package-local
// for the same reason it keeps its own: document editing is self-contained,
// and a shared helper package would couple three unrelated file formats.

func nodeFor(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

// Replacing only the value preserves the key's head comment. Its anchor moves
// too so existing aliases keep resolving.
func setNode(mapping *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			value.Anchor = mapping.Content[i+1].Anchor
			mapping.Content[i+1] = value
			return
		}
	}
	mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, value)
}

func setScalar(mapping *yaml.Node, key string, value any) error {
	var v yaml.Node
	if err := v.Encode(value); err != nil {
		return fmt.Errorf("encode %s: %w", key, err)
	}
	setNode(mapping, key, &v)
	return nil
}

func removeKey(mapping *yaml.Node, key string) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
			return
		}
	}
}

func encodeDoc(doc *yaml.Node) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, fmt.Errorf("encode the Hive config: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("encode the Hive config: %w", err)
	}
	return buf.Bytes(), nil
}

// writeFileAtomic writes via temp-file-then-rename so a crash mid-write never
// leaves a half-written config for the next launch to fail on. 0o644, not the
// 0o600 the desktop's own files use: this file is shared with the `hive` CLI
// and carries no secret.
func writeFileAtomic(path string, data []byte, check Check) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil { //nolint:gosec // shared with the hive CLI, holds no secret
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	if check != nil {
		if err := check(tmp); err != nil {
			_ = os.Remove(tmp)
			return invalid("Hive would not load the result: %v", err)
		}
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace %s: %w", filepath.Base(path), err)
	}
	return nil
}
