package flow

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// This file holds small, generic yaml.Node tree helpers used by SaveFlow to
// edit an existing flow document in place. They stay in package flow because
// flow document editing is deliberately self-contained (see the package doc
// in flow.go).

// parseDocNode parses data into its node tree and returns the document node
// plus its root mapping.
func parseDocNode(data []byte) (doc, root *yaml.Node, err error) {
	doc = &yaml.Node{}
	if err := yaml.Unmarshal(data, doc); err != nil {
		return nil, nil, fmt.Errorf("flow: parse document: %w", err)
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, nil, fmt.Errorf("flow: document is not a mapping")
	}
	return doc, doc.Content[0], nil
}

// setOrAddNode sets mapping[key] = value, appending a new key/value pair
// when key is not already present.
func setOrAddNode(mapping *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content[i+1] = value
			return
		}
	}
	mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, value)
}

// setOrAddScalar encodes value and sets it at mapping[key].
func setOrAddScalar(mapping *yaml.Node, key string, value any) error {
	var v yaml.Node
	if err := v.Encode(value); err != nil {
		return fmt.Errorf("flow: encode %s: %w", key, err)
	}
	setOrAddNode(mapping, key, &v)
	return nil
}

// removeMappingKey removes key from mapping, reporting whether it was
// present.
func removeMappingKey(mapping *yaml.Node, key string) bool {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
			return true
		}
	}
	return false
}

// encodeDoc renders a parsed document node tree back to bytes, preserving
// whatever comments and formatting remain attached to it.
func encodeDoc(doc *yaml.Node) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, fmt.Errorf("flow: encode document: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("flow: encode document: %w", err)
	}
	return buf.Bytes(), nil
}

// writeFileAtomic writes data to path via a temp-file-then-rename, so a
// crash mid-write never leaves a half-written flow/layout file — the same
// pattern as internal/desktop/feed/store.go's writeFileAtomic, duplicated
// here to keep package flow decoupled from feed.
func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("flow: create %s dir: %w", filepath.Base(path), err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("flow: write %s: %w", filepath.Base(path), err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("flow: replace %s: %w", filepath.Base(path), err)
	}
	return nil
}
