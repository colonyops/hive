package drive

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	frontmatterFence = "---"
	utf8BOM          = "\xef\xbb\xbf"
)

// ParseMarkdownFrontmatter reads the YAML block between a leading "---" line
// and the next "---" line from the head of a markdown file. present is true
// only when such a block was parsed; a head that opens a block and never
// closes it (the block runs past the bytes read, or the file is cut short)
// is an ErrInvalidFrontmatter with present false. Values are kept as YAML
// parses them, except that dates and timestamps stay the text the author
// wrote.
func ParseMarkdownFrontmatter(head []byte) (fm Frontmatter, present bool, err error) {
	text := string(bytes.ReplaceAll(bytes.TrimPrefix(head, []byte(utf8BOM)), []byte("\r\n"), []byte("\n")))
	if !strings.HasPrefix(text, frontmatterFence+"\n") {
		return nil, false, nil
	}
	body := text[len(frontmatterFence):]
	end := closingFence(body)
	if end < 0 {
		return nil, false, fmt.Errorf("%w: unterminated front matter", ErrInvalidFrontmatter)
	}
	header := strings.TrimPrefix(body[:end], "\n")
	if strings.TrimSpace(header) == "" {
		return Frontmatter{}, true, nil
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(header), &doc); err != nil {
		return nil, false, fmt.Errorf("%w: %w", ErrInvalidFrontmatter, err)
	}
	keepTimestampText(&doc)
	// Decoding into the named type would make every nested mapping a
	// Frontmatter too; a plain map keeps them map[string]any for consumers.
	var values map[string]any
	if err := doc.Decode(&values); err != nil {
		return nil, false, fmt.Errorf("%w: %w", ErrInvalidFrontmatter, err)
	}
	return Frontmatter(values), true, nil
}

// closingFence finds "\n---" ending a line in body, where body starts at the
// newline that closed the opening fence; -1 when there is none. Spaces and
// tabs after the fence still end it, since an editor can leave them there.
func closingFence(body string) int {
	from := 0
	for {
		i := strings.Index(body[from:], "\n"+frontmatterFence)
		if i < 0 {
			return -1
		}
		i += from
		after := i + len("\n"+frontmatterFence)
		for after < len(body) && (body[after] == ' ' || body[after] == '\t') {
			after++
		}
		if after == len(body) || body[after] == '\n' {
			return i
		}
		from = after
	}
}

// keepTimestampText retags YAML dates as strings so "date: 2026-10-11"
// decodes as the text written rather than a time.Time at midnight UTC.
func keepTimestampText(n *yaml.Node) {
	if n.Kind == yaml.ScalarNode && n.Tag == "!!timestamp" {
		n.Tag = "!!str"
	}
	for _, child := range n.Content {
		keepTimestampText(child)
	}
}
