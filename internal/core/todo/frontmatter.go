package todo

import (
	"bufio"
	"strings"

	"gopkg.in/yaml.v3"
)

// Frontmatter holds metadata parsed from a document's YAML front matter.
// All fields are best-effort: missing or malformed frontmatter produces zero values.
type Frontmatter struct {
	SessionID string `yaml:"session_id"`
	Title     string `yaml:"title"`
}

// ParseFrontmatter extracts YAML front matter from document content.
// Front matter must be delimited by "---" on its own line at the start of the file.
// Returns zero-value Frontmatter if no valid front matter is found.
func ParseFrontmatter(content string) Frontmatter {
	scanner := bufio.NewScanner(strings.NewReader(content))

	// First line must be "---"
	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != "---" {
		return Frontmatter{}
	}

	var lines []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			break
		}
		lines = append(lines, line)
	}

	if len(lines) == 0 {
		return Frontmatter{}
	}

	var fm Frontmatter
	_ = yaml.Unmarshal([]byte(strings.Join(lines, "\n")), &fm)

	return fm
}
