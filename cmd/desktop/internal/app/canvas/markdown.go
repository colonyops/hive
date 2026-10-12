// Package canvas renders canvas documents for the desktop: the html
// sanitizer the pane and the export run through, and the markdown export.
// The model and the files are the engine's drive service.
package canvas

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/colonyops/hive/internal/domain/drive"
)

// Markdown renders a standalone document: YAML front matter with the canvas
// timestamps and the agent's metadata, the title, then every block. An html
// block leaves sanitized, because the file is opened elsewhere by something
// with no policy of its own.
func Markdown(c drive.Canvas) (string, error) {
	header, err := markdownFrontmatter(c)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString("---\n")
	b.Write(header)
	b.WriteString("---\n\n")
	if c.Title != "" {
		b.WriteString("# " + c.Title + "\n\n")
	}
	for i, block := range c.Blocks {
		if i > 0 {
			b.WriteString("\n\n")
		}
		switch block.Kind {
		case drive.BlockKindLink:
			b.WriteString("[" + block.Title + "](" + block.URL + ")")
		case drive.BlockKindHTML:
			if block.Title != "" {
				b.WriteString("## " + block.Title + "\n\n")
			}
			b.WriteString(SanitizeHTML(block.Body))
		default:
			if block.Title != "" {
				b.WriteString("## " + block.Title + "\n\n")
			}
			b.WriteString(block.Body)
		}
	}
	b.WriteString("\n")
	return b.String(), nil
}

func markdownFrontmatter(c drive.Canvas) ([]byte, error) {
	if err := drive.ValidateFrontmatter(c.Frontmatter); err != nil {
		return nil, err
	}
	root := yaml.Node{Kind: yaml.MappingNode}
	appendYAMLPair := func(key string, value any) error {
		keyNode := yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
		var valueNode yaml.Node
		if err := valueNode.Encode(value); err != nil {
			return fmt.Errorf("encode front matter %q: %w", key, err)
		}
		root.Content = append(root.Content, &keyNode, &valueNode)
		return nil
	}
	if err := appendYAMLPair("created_at", time.UnixMilli(c.CreatedAt).UTC().Format(time.RFC3339Nano)); err != nil {
		return nil, err
	}
	if err := appendYAMLPair("updated_at", time.UnixMilli(c.UpdatedAt).UTC().Format(time.RFC3339Nano)); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(c.Frontmatter))
	for key := range c.Frontmatter {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := appendYAMLPair(key, c.Frontmatter[key]); err != nil {
			return nil, err
		}
	}
	return yaml.Marshal(&root)
}
