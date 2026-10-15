package core_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/internal/domain/drive"
)

func TestDeriveTitle(t *testing.T) {
	tests := []struct {
		name string
		path string
		kind drive.Kind
		body string
		want string
	}{
		{"frontmatter wins over heading", "a/notes.md", drive.KindMarkdown, "---\ntitle: From Meta\n---\n# Heading\n", "From Meta"},
		{"heading", "notes.md", drive.KindMarkdown, "intro\n\n# The Heading #\n\ntext", "The Heading"},
		{"heading after frontmatter without title", "notes.md", drive.KindMarkdown, "---\n# yaml comment\ntags: [a]\n---\n# Real\n", "Real"},
		{"heading in code fence ignored", "notes.md", drive.KindMarkdown, "```\n# not a title\n```\n", "notes"},
		{"level two heading ignored", "plan.md", drive.KindMarkdown, "## Sub\n", "plan"},
		{"blank frontmatter title falls through", "x.md", drive.KindMarkdown, "---\ntitle: \"  \"\n---\n# H\n", "H"},
		{"unterminated frontmatter", "x.md", drive.KindMarkdown, "---\ntitle: T\n# H\n", "H"},
		{"html title", "page.html", drive.KindHtml, "<html><head><TITLE>\n Q3  &amp; Q4 </TITLE></head></html>", "Q3 & Q4"},
		{"html without title", "dir/page.html", drive.KindHtml, "<h1>x</h1>", "page"},
		{"fallback keeps inner dots", "dir/v1.2.notes.md", drive.KindMarkdown, "", "v1.2.notes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, core.DeriveTitle(tt.path, tt.kind, []byte(tt.body)))
		})
	}
}
