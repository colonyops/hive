package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// SetAnalyticsEnabled edits only collection gates and preserves unrelated configuration.
func SetAnalyticsEnabled(path string, enabled bool, check Check) error {
	if path == "" {
		return fmt.Errorf("no Hive config path is available")
	}
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read Hive config: %w", err)
	}
	if target, err := filepath.EvalSymlinks(path); err == nil {
		path = target
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	doc := &yaml.Node{}
	if err := yaml.Unmarshal(raw, doc); err != nil {
		return fmt.Errorf("parse Hive config: %w", err)
	}
	if len(doc.Content) == 0 {
		doc = &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return invalid("the Hive config is not a YAML mapping")
	}
	analytics, err := analyticsMapping(root, "analytics")
	if err != nil {
		return err
	}
	if err := setScalar(analytics, "enabled", enabled); err != nil {
		return err
	}
	if enabled {
		local, err := analyticsMapping(analytics, "local")
		if err != nil {
			return err
		}
		if err := setScalar(local, "enabled", true); err != nil {
			return err
		}
	}
	candidate, err := encodeDoc(doc)
	if err != nil {
		return err
	}
	return writeFileAtomic(path, candidate, check)
}

func analyticsMapping(parent *yaml.Node, key string) (*yaml.Node, error) {
	node := nodeFor(parent, key)
	if node == nil {
		node = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		setNode(parent, key, node)
	}
	if node.Kind != yaml.MappingNode {
		return nil, invalid("%s must be a mapping", key)
	}
	return node, nil
}
