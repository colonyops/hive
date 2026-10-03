package agentws

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"

	configmigrate "github.com/colonyops/hive/internal/config/migrate"
)

// ErrLibraryServerExists reports an AddLibraryServers id already declared in
// mcps.yaml. Overwriting a declared server from a paste would silently replace
// configuration shared by every workspace, so a collision is refused and the
// file is the place to change an existing entry.
var ErrLibraryServerExists = errors.New("agentws: server already declared in mcps.yaml")

// AddLibraryServers inserts servers into <root>/mcps.yaml, editing the parsed
// node tree in place (the WriteManifest pattern) so comments, key order, and
// keys this build does not know survive. A missing file is created fresh;
// nothing is written when any id collides with a declared server.
func AddLibraryServers(root string, servers map[string]MCPServer) error {
	path := filepath.Join(root, libraryFileName)

	var doc, mapping *yaml.Node
	raw, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		mapping = &yaml.Node{Kind: yaml.MappingNode}
		doc = &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{mapping}}
		var version yaml.Node
		if err := version.Encode(configmigrate.MCPLibrarySet.Current); err != nil {
			return fmt.Errorf("mcps.yaml: encode version: %w", err)
		}
		mapping.Content = append(mapping.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "version"}, &version)
	case err != nil:
		return fmt.Errorf("mcps.yaml: %w", err)
	default:
		doc, mapping, err = parseLibraryNode(raw)
		if err != nil {
			return err
		}
	}

	serversNode := libraryServersNode(mapping)
	declared := make(map[string]bool, len(serversNode.Content)/2)
	for i := 0; i+1 < len(serversNode.Content); i += 2 {
		declared[serversNode.Content[i].Value] = true
	}

	ids := make([]string, 0, len(servers))
	for id := range servers {
		if declared[id] {
			return fmt.Errorf("%w: %q", ErrLibraryServerExists, id)
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		var v yaml.Node
		if err := v.Encode(servers[id]); err != nil {
			return fmt.Errorf("mcps.yaml: encode server %q: %w", id, err)
		}
		serversNode.Content = append(serversNode.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: id}, &v)
	}

	out, err := encodeManifestDoc(doc)
	if err != nil {
		return fmt.Errorf("mcps.yaml: %w", err)
	}
	if err := writeFileAtomic(path, out); err != nil {
		return fmt.Errorf("mcps.yaml: %w", err)
	}
	return nil
}

// RemoveLibraryServer deletes id from <root>/mcps.yaml's servers map, editing
// the document in place. found reports whether the id was declared at all; a
// missing file or absent servers key is simply not found, never an error.
func RemoveLibraryServer(root, id string) (found bool, err error) {
	path := filepath.Join(root, libraryFileName)
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("mcps.yaml: %w", err)
	}
	doc, mapping, err := parseLibraryNode(raw)
	if err != nil {
		return false, err
	}

	serversNode := findMappingValue(mapping, "servers")
	if serversNode == nil || serversNode.Kind != yaml.MappingNode {
		return false, nil
	}
	for i := 0; i+1 < len(serversNode.Content); i += 2 {
		if serversNode.Content[i].Value != id {
			continue
		}
		serversNode.Content = append(serversNode.Content[:i], serversNode.Content[i+2:]...)
		out, err := encodeManifestDoc(doc)
		if err != nil {
			return false, fmt.Errorf("mcps.yaml: %w", err)
		}
		if bytes.Equal(raw, out) {
			return true, nil
		}
		if err := writeFileAtomic(path, out); err != nil {
			return false, fmt.Errorf("mcps.yaml: %w", err)
		}
		return true, nil
	}
	return false, nil
}

func parseLibraryNode(data []byte) (doc, mapping *yaml.Node, err error) {
	doc = &yaml.Node{}
	if err := yaml.Unmarshal(data, doc); err != nil {
		return nil, nil, fmt.Errorf("mcps.yaml: parse: %w", err)
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, nil, fmt.Errorf("mcps.yaml: document is not a mapping")
	}
	return doc, doc.Content[0], nil
}

// libraryServersNode returns the block mapping under the servers key, creating
// or converting as needed: the seeded file says `servers: {}` (a flow mapping)
// and a null or flow value must become a block mapping before entries land in
// it, or the whole set renders inline on one line.
func libraryServersNode(mapping *yaml.Node) *yaml.Node {
	node := findMappingValue(mapping, "servers")
	if node == nil {
		node = &yaml.Node{Kind: yaml.MappingNode}
		mapping.Content = append(mapping.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "servers"}, node)
		return node
	}
	if node.Kind != yaml.MappingNode {
		*node = yaml.Node{Kind: yaml.MappingNode}
		return node
	}
	node.Style = 0
	return node
}

func findMappingValue(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}
