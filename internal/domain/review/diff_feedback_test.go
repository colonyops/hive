package review

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDiffFeedbackIsEmptyWithNothingToSay(t *testing.T) {
	assert.Empty(t, DiffFeedback(nil))
}

func TestDiffFeedbackOrdersByPathAndLine(t *testing.T) {
	got := DiffFeedback([]DiffComment{
		{Path: "b.go", StartSide: DiffSideAdditions, StartLine: 3, EndSide: DiffSideAdditions, EndLine: 3, Quote: "+x := 1", Text: "rename"},
		{Path: "a.go", StartSide: DiffSideAdditions, StartLine: 9, EndSide: DiffSideAdditions, EndLine: 12, Quote: "+y", Text: "two\nlines"},
	})

	assert.Equal(t, `# Code Review Feedback

I reviewed the changes in this session and have 2 pieces of feedback:

## 1. a.go (lines 9-12)
`+"```\n+y\n```"+`
> two
> lines

## 2. b.go (line 3)
`+"```\n+x := 1\n```"+`
> rename
`, got)
}

func TestDiffFeedbackNamesRemovedLines(t *testing.T) {
	cases := map[string]DiffComment{
		"(removed line 4)":           {StartSide: DiffSideDeletions, StartLine: 4, EndSide: DiffSideDeletions, EndLine: 4},
		"(removed lines 4-6)":        {StartSide: DiffSideDeletions, StartLine: 4, EndSide: DiffSideDeletions, EndLine: 6},
		"(removed line 4 to line 5)": {StartSide: DiffSideDeletions, StartLine: 4, EndSide: DiffSideAdditions, EndLine: 5},
		"(line 5 to removed line 9)": {StartSide: DiffSideAdditions, StartLine: 5, EndSide: DiffSideDeletions, EndLine: 9},
	}
	for label, comment := range cases {
		comment.Path, comment.Text = "a.go", "why"
		got := DiffFeedback([]DiffComment{comment})
		assert.Contains(t, got, "a.go "+label)
		assert.Contains(t, got, "Removed lines are numbered")
	}
}

func TestDiffFeedbackOmitsTheNumberingNoteWithoutRemovedLines(t *testing.T) {
	got := DiffFeedback([]DiffComment{{Path: "a.go", StartSide: DiffSideAdditions, StartLine: 1, EndSide: DiffSideAdditions, EndLine: 1, Text: "why"}})
	assert.NotContains(t, got, "Removed lines")
	assert.NotContains(t, got, "```")
}

func TestDiffFeedbackFenceOutlastsBackticksInTheQuote(t *testing.T) {
	got := DiffFeedback([]DiffComment{{Path: "a.md", Quote: "+```go\n+````", Text: "why"}})
	assert.Contains(t, got, "`````\n+```go\n+````\n`````\n")
}

func TestDiffFeedbackCutsLongQuotes(t *testing.T) {
	lines := make([]string, maxQuotedDiffLines+5)
	for i := range lines {
		lines[i] = fmt.Sprintf("+line %d", i)
	}
	got := DiffFeedback([]DiffComment{{Path: "a.go", Quote: strings.Join(lines, "\n"), Text: "why"}})
	assert.Contains(t, got, fmt.Sprintf("+line %d\n", maxQuotedDiffLines-1))
	assert.NotContains(t, got, fmt.Sprintf("+line %d\n", maxQuotedDiffLines))
	assert.Contains(t, got, "... (5 more lines)")
}
