package skills

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleSkill() Skill {
	return Skill{
		ID:          "flows",
		Name:        "hive-flows",
		Description: `Author or edit a flow: the "graph" of sources and destinations.`,
		Body:        "# Flows\n\nEdit /home/u/.config/hive/desktop/flows/<id>.yaml.\n",
	}
}

func TestRenderProducesValidFrontmatter(t *testing.T) {
	content, err := Render(sampleSkill())
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(content, "---\nname: hive-flows\n"), "frontmatter opens with the slug:\n%s", content)
	// The description carries a colon and a quote; both must be escaped so the
	// YAML frontmatter still parses.
	assert.Contains(t, content, `description: "Author or edit a flow: the \"graph\" of sources and destinations."`)
	assert.Contains(t, content, "# Flows")
	assert.True(t, strings.HasSuffix(content, "\n"))
}

func TestValidateSkill(t *testing.T) {
	require.NoError(t, ValidateSkill(sampleSkill()))

	cases := map[string]func(Skill) Skill{
		"empty id":        func(s Skill) Skill { s.ID = ""; return s },
		"empty name":      func(s Skill) Skill { s.Name = ""; return s },
		"uppercase name":  func(s Skill) Skill { s.Name = "Hive-Flows"; return s },
		"reserved word":   func(s Skill) Skill { s.Name = "claude-helper"; return s },
		"trailing hyphen": func(s Skill) Skill { s.Name = "hive-flows-"; return s },
		"no description":  func(s Skill) Skill { s.Description = "  "; return s },
		"empty body":      func(s Skill) Skill { s.Body = ""; return s },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Error(t, ValidateSkill(mutate(sampleSkill())))
		})
	}
}
