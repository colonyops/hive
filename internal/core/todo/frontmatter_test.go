package todo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseFrontmatter(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    Frontmatter
	}{
		{
			name: "valid frontmatter with all fields",
			content: `---
session_id: abc123
title: My Plan
---
# Document body
`,
			want: Frontmatter{SessionID: "abc123", Title: "My Plan"},
		},
		{
			name: "valid frontmatter with session_id only",
			content: `---
session_id: sess-xyz
---
content here
`,
			want: Frontmatter{SessionID: "sess-xyz"},
		},
		{
			name:    "no frontmatter",
			content: "# Just a heading\nSome content\n",
			want:    Frontmatter{},
		},
		{
			name:    "empty content",
			content: "",
			want:    Frontmatter{},
		},
		{
			name: "frontmatter without closing delimiter",
			content: `---
session_id: orphaned
`,
			want: Frontmatter{SessionID: "orphaned"},
		},
		{
			name: "frontmatter with extra fields ignored",
			content: `---
session_id: s1
title: Title
author: someone
priority: high
---
body
`,
			want: Frontmatter{SessionID: "s1", Title: "Title"},
		},
		{
			name:    "delimiter not on first line",
			content: "\n---\nsession_id: nope\n---\n",
			want:    Frontmatter{},
		},
		{
			name: "empty frontmatter block",
			content: `---
---
content
`,
			want: Frontmatter{},
		},
		{
			name: "frontmatter with whitespace around delimiters",
			content: `  ---
session_id: ws
  ---
body
`,
			want: Frontmatter{SessionID: "ws"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseFrontmatter(tt.content)
			assert.Equal(t, tt.want, got)
		})
	}
}
