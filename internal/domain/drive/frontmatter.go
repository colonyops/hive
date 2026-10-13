package drive

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Canvas front matter limits: room for any property list a reader shows in
// full, and too small for an agent to use front matter as a second document.
const (
	maxFrontmatterKeys     = 50
	maxFrontmatterKeyBytes = 64
	maxFrontmatterText     = 4096
	maxFrontmatterList     = 50
	maxFrontmatterBytes    = 64 * 1024
)

var frontmatterKeyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

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
// wrote; validation applies only to canvas writes.
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

// ValidateFrontmatter accepts a flat mapping whose values are scalars or
// scalar lists, which is what a canvas may carry: the reader presents it as
// a compact property list, not a second document. created_at and updated_at
// are Hive's own.
func ValidateFrontmatter(fm Frontmatter) error {
	if len(fm) > maxFrontmatterKeys {
		return fmt.Errorf("%w: too many fields (%d max)", ErrInvalidFrontmatter, maxFrontmatterKeys)
	}
	for key, value := range fm {
		if len(key) > maxFrontmatterKeyBytes || !frontmatterKeyPattern.MatchString(key) {
			return fmt.Errorf("%w: key %q must use letters, digits, dots, hyphens, or underscores", ErrInvalidFrontmatter, key)
		}
		if strings.EqualFold(key, "created_at") || strings.EqualFold(key, "updated_at") {
			return fmt.Errorf("%w: %q is managed by Hive", ErrInvalidFrontmatter, key)
		}
		if err := validateFrontmatterValue(value, false); err != nil {
			return fmt.Errorf("%w: %q: %w", ErrInvalidFrontmatter, key, err)
		}
	}
	data, err := json.Marshal(fm)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidFrontmatter, err)
	}
	if len(data) > maxFrontmatterBytes {
		return fmt.Errorf("%w: metadata is too large (%d bytes max)", ErrInvalidFrontmatter, maxFrontmatterBytes)
	}
	return nil
}

func validateFrontmatterValue(value any, inList bool) error {
	switch value := value.(type) {
	case nil, bool, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return nil
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return errors.New("number must be finite")
		}
		return nil
	case string:
		if len(value) > maxFrontmatterText {
			return fmt.Errorf("text is too long (%d bytes max)", maxFrontmatterText)
		}
		return nil
	case []any:
		if inList {
			return errors.New("nested lists are not allowed")
		}
		if len(value) > maxFrontmatterList {
			return fmt.Errorf("list has too many values (%d max)", maxFrontmatterList)
		}
		for _, item := range value {
			if err := validateFrontmatterValue(item, true); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("value has unsupported type %T; use a scalar or a list of scalars", value)
	}
}
