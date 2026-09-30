// Package skills renders Hive Desktop's paste-ready prompts in the Agent Skills
// format: a SKILL.md with YAML frontmatter (name, description) over the prompt
// text. It owns the format's rules — the naming constraints a target agent
// enforces and the frontmatter template — and nothing else. Where a rendered
// skill is written, and by whom, is the workspace generator's business
// (internal/app/agentws, ADR skills-are-declared-by-a-workspace).
package skills

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"text/template"
)

//go:embed templates/*.tmpl
var templatesFS embed.FS

var skillTmpl = template.Must(template.New("skills").Funcs(funcs()).ParseFS(templatesFS, "templates/*.tmpl")).Lookup("skill.tmpl")

// Skill is the content a SKILL.md carries, already rendered by the prompt
// engine. Name is the on-disk slug and, per the Agent Skills standard, must
// equal the directory the SKILL.md lives in.
type Skill struct {
	ID          string // stable prompt id, e.g. "flows"
	Name        string // namespaced slug and directory name, e.g. "hive-flows"
	Description string // frontmatter description: what the skill does and when
	Body        string // the rendered prompt text
}

// Render produces the SKILL.md contents for a skill.
func Render(s Skill) (string, error) {
	var buf bytes.Buffer
	if err := skillTmpl.Execute(&buf, s); err != nil {
		return "", fmt.Errorf("skills: render template: %w", err)
	}
	return strings.TrimSpace(buf.String()) + "\n", nil
}

func funcs() template.FuncMap {
	return template.FuncMap{
		// yaml renders a value as a safe single-line double-quoted YAML scalar,
		// so a description containing a colon or quote cannot break the SKILL.md
		// frontmatter it sits in.
		"yaml": yamlScalar,
	}
}

func yamlScalar(s string) string {
	s = strings.ReplaceAll(s, "\\", `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", " ")
	return `"` + s + `"`
}

var nameRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ValidateSkill enforces the Agent Skills naming rules before a file is written,
// so a render never produces a SKILL.md an agent would reject.
func ValidateSkill(s Skill) error {
	if s.ID == "" {
		return errors.New("skills: skill id is required")
	}
	if l := len(s.Name); l == 0 || l > 64 {
		return fmt.Errorf("skills: skill name %q must be 1-64 characters", s.Name)
	}
	if !nameRe.MatchString(s.Name) {
		return fmt.Errorf("skills: skill name %q must be lowercase letters, digits and single hyphens", s.Name)
	}
	if strings.Contains(s.Name, "anthropic") || strings.Contains(s.Name, "claude") {
		return fmt.Errorf("skills: skill name %q must not contain a reserved word", s.Name)
	}
	if strings.TrimSpace(s.Description) == "" {
		return errors.New("skills: skill description is required")
	}
	if len(s.Description) > 1024 {
		return errors.New("skills: skill description must be at most 1024 characters")
	}
	if strings.TrimSpace(s.Body) == "" {
		return errors.New("skills: skill body is empty")
	}
	return nil
}
