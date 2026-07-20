package flow

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// flowFileHeader opens a freshly created flows/*.yaml document. It is only
// written once, when the file doesn't exist yet — an edit of an existing
// file never touches it, which is how the header survives repeated saves.
const flowFileHeader = `# Hive Desktop flow — nodes and wires, as code.
# Edited by hand or by the app; changes apply on Deploy (the frontend graph
# runtime drains in-flight messages, then swaps in the reloaded graph).
`

// SaveFlow writes f to path (flows/<id>.yaml). The frontend never
// serializes YAML itself: it sends a Flow value and this is the only place
// that turns it into a document, matching feed's one-way "Go owns the
// on-disk format" convention.
//
// If path doesn't exist (or is empty), a clean document is marshaled with a
// short header comment. If it exists, its yaml.Node tree is edited in
// place: version/name/enabled are set as scalars on the existing root
// mapping, and the nodes/wires sequences are replaced wholesale. This
// preserves the document's leading header comment and any comments on
// unrelated top-level keys, but — since there's no way to tell, from a
// Flow value alone, which individual node or wire actually changed —
// comments attached to specific node/wire entries do not survive an edit.
// Full per-entry comment retention is future work if it turns out to
// matter in practice; round-trip structural fidelity (LoadFlow -> SaveFlow
// -> LoadFlow yields an equal Flow) does not depend on it.
func SaveFlow(path string, f Flow) error {
	if id := flowIDFromFilename(filepath.Base(path)); id != f.ID {
		return fmt.Errorf("flow: path %q does not match flow id %q", path, f.ID)
	}

	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("flow: read %s: %w", filepath.Base(path), err)
	}

	var data []byte
	if len(bytes.TrimSpace(existing)) == 0 {
		data, err = newFlowDocument(f)
	} else {
		data, err = editFlowDocument(existing, f)
	}
	if err != nil {
		return err
	}
	return writeFileAtomic(path, data)
}

// newFlowDocument marshals f as a clean document with a header comment, for
// a flow file that doesn't exist yet (or was empty).
func newFlowDocument(f Flow) ([]byte, error) {
	root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	if err := populateFlowMapping(root, f); err != nil {
		return nil, err
	}
	doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}
	out, err := encodeDoc(doc)
	if err != nil {
		return nil, err
	}
	return append([]byte(flowFileHeader), out...), nil
}

// editFlowDocument edits an existing document's node tree in place: version,
// name, and enabled are set/removed as scalars on the root mapping, and the
// nodes/wires sequences are replaced wholesale. Only the specific keys this
// touches are affected — the document's header comment (attached to the
// root node, never replaced) and any comments on other top-level keys
// survive.
func editFlowDocument(data []byte, f Flow) ([]byte, error) {
	doc, root, err := parseDocNode(data)
	if err != nil {
		return nil, err
	}
	if err := populateFlowMapping(root, f); err != nil {
		return nil, err
	}
	return encodeDoc(doc)
}

// populateFlowMapping sets f's fields onto root as flowFile's on-disk keys
// would appear: version always present, name/enabled/wires present only
// when non-default, nodes always present (even when empty, so an
// intentionally cleared flow still round-trips as `nodes: []` rather than a
// missing key).
func populateFlowMapping(root *yaml.Node, f Flow) error {
	if err := setOrAddScalar(root, "version", 1); err != nil {
		return err
	}

	if f.Name != "" {
		if err := setOrAddScalar(root, "name", f.Name); err != nil {
			return err
		}
	} else {
		removeMappingKey(root, "name")
	}

	if f.Enabled {
		removeMappingKey(root, "enabled")
	} else if err := setOrAddScalar(root, "enabled", false); err != nil {
		return err
	}

	nodesSeq, err := encodeNodesSeq(f.Nodes)
	if err != nil {
		return err
	}
	setOrAddNode(root, "nodes", nodesSeq)

	if len(f.Wires) > 0 {
		wiresSeq, err := encodeWiresSeq(f.Wires)
		if err != nil {
			return err
		}
		setOrAddNode(root, "wires", wiresSeq)
	} else {
		removeMappingKey(root, "wires")
	}

	return nil
}

func encodeNodesSeq(nodes []Node) (*yaml.Node, error) {
	seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for i := range nodes {
		var entry yaml.Node
		if err := entry.Encode(nodes[i]); err != nil {
			return nil, fmt.Errorf("flow: encode node %q: %w", nodes[i].ID, err)
		}
		seq.Content = append(seq.Content, &entry)
	}
	return seq, nil
}

func encodeWiresSeq(wires []Wire) (*yaml.Node, error) {
	seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for i := range wires {
		var entry yaml.Node
		if err := entry.Encode(wires[i]); err != nil {
			return nil, fmt.Errorf("flow: encode wire %s->%s: %w", wires[i].From, wires[i].To, err)
		}
		seq.Content = append(seq.Content, &entry)
	}
	return seq, nil
}
