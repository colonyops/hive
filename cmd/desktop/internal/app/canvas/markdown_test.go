package canvas

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/domain/drive"
)

func TestMarkdownRendersFrontmatterTitleBlocksAndLinks(t *testing.T) {
	c := drive.Canvas{
		Title: "The Plan", CreatedAt: 1_700_000_000_000, UpdatedAt: 1_700_000_060_000,
		Frontmatter: drive.Frontmatter{"tags": []any{"release", "desktop"}, "reviewed": true},
		Blocks: []drive.Block{
			{ID: "intro", Kind: drive.BlockKindMarkdown, Title: "Intro", Body: "hello"},
			{ID: "body", Kind: drive.BlockKindMarkdown, Body: "world"},
			{ID: "pr", Kind: drive.BlockKindLink, Title: "The PR", URL: "https://example.com/pr/1"},
		},
	}
	want := "---\ncreated_at: \"2023-11-14T22:13:20Z\"\nupdated_at: \"2023-11-14T22:14:20Z\"\nreviewed: true\ntags:\n    - release\n    - desktop\n---\n\n# The Plan\n\n## Intro\n\nhello\n\nworld\n\n[The PR](https://example.com/pr/1)\n"
	markdown, err := Markdown(c)
	require.NoError(t, err)
	assert.Equal(t, want, markdown)

	markdown, err = Markdown(drive.Canvas{CreatedAt: 1, UpdatedAt: 2, Blocks: []drive.Block{{Kind: drive.BlockKindMarkdown, Body: "hello"}}})
	require.NoError(t, err)
	assert.Equal(t, "---\ncreated_at: \"1970-01-01T00:00:00.001Z\"\nupdated_at: \"1970-01-01T00:00:00.002Z\"\n---\n\nhello\n", markdown,
		"no canvas title means no heading")
}

func TestMarkdownPreservesMermaidFence(t *testing.T) {
	body := "Before\n\n```mermaid\nflowchart LR\nA --> B\n```\n\nAfter"
	markdown, err := Markdown(drive.Canvas{CreatedAt: 1, UpdatedAt: 2, Blocks: []drive.Block{{Kind: drive.BlockKindMarkdown, Body: body}}})
	require.NoError(t, err)
	assert.Contains(t, markdown, body+"\n")
}

// An export leaves the app, so it carries the sanitized markup rather than
// what the agent wrote: the file is opened elsewhere, by something with no
// policy of its own.
func TestMarkdownEmitsSanitizedHTMLBlocks(t *testing.T) {
	c := drive.Canvas{
		Blocks: []drive.Block{
			{ID: "stats", Kind: drive.BlockKindHTML, Title: "Run", Body: `<p class="hv-muted">green</p><script>alert(1)</script>`},
		},
	}
	markdown, err := Markdown(c)
	require.NoError(t, err)
	assert.Contains(t, markdown, "## Run\n\n<p class=\"hv-muted\">green</p>\n")
}

func TestMarkdownRefusesInvalidFrontmatter(t *testing.T) {
	_, err := Markdown(drive.Canvas{Frontmatter: drive.Frontmatter{"created_at": "yesterday"}})
	require.ErrorIs(t, err, drive.ErrInvalidFrontmatter)
}

func TestMarkdownEscapesTitlesAndLinkDestinations(t *testing.T) {
	c := drive.Canvas{
		Title: "Line one\nline two", CreatedAt: 1, UpdatedAt: 2,
		Blocks: []drive.Block{
			{ID: "a", Kind: drive.BlockKindMarkdown, Title: "Part\r\nTwo", Body: "x"},
			{ID: "b", Kind: drive.BlockKindLink, Title: "See [docs]\nhere", URL: "https://example.com/a_(b)"},
			{ID: "c", Kind: drive.BlockKindLink, Title: "Space", URL: "https://example.com/a b"},
			{ID: "d", Kind: drive.BlockKindLink, Title: "Plain", URL: "https://example.com/a"},
		},
	}
	markdown, err := Markdown(c)
	require.NoError(t, err)
	assert.Contains(t, markdown, "# Line one line two\n\n")
	assert.Contains(t, markdown, "## Part Two\n\n")
	assert.Contains(t, markdown, `[See \[docs\] here](<https://example.com/a_(b)>)`)
	assert.Contains(t, markdown, "[Space](<https://example.com/a b>)")
	assert.Contains(t, markdown, "[Plain](https://example.com/a)")
}
