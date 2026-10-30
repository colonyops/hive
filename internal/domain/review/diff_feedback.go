package review

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

// DiffSide says which version of a file a diff line number belongs to.
type DiffSide string

const (
	DiffSideAdditions DiffSide = "additions"
	DiffSideDeletions DiffSide = "deletions"
)

// maxQuotedDiffLines keeps one long selection from burying the comments around
// it. The agent has the file; the quote only has to identify the lines.
const maxQuotedDiffLines = 30

// DiffComment is feedback on a run of rows in one file's diff. The run starts
// and ends on a side each, because a unified diff can select from a removed
// line into an added one.
type DiffComment struct {
	Path      string
	StartSide DiffSide
	StartLine int
	EndSide   DiffSide
	EndLine   int
	Quote     string
	Text      string
}

// DiffFeedback formats comments on a session's changes as the text its agent
// reads, by path and line. It returns "" when there are no comments.
//
// Format:
//
//	# Code Review Feedback
//
//	I reviewed the changes in this session and have <n> pieces of feedback:
//
//	## 1. <path> (lines <start>-<end>)
//	```
//	<quote>
//	```
//	> <comment>
func DiffFeedback(comments []DiffComment) string {
	count := len(comments)
	if count == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("# Code Review Feedback\n\n")
	if count == 1 {
		b.WriteString("I reviewed the changes in this session and have 1 piece of feedback:\n")
	} else {
		fmt.Fprintf(&b, "I reviewed the changes in this session and have %d pieces of feedback:\n", count)
	}
	if slices.ContainsFunc(comments, func(c DiffComment) bool {
		return c.StartSide == DiffSideDeletions || c.EndSide == DiffSideDeletions
	}) {
		b.WriteString("\nRemoved lines are numbered as in the version the diff started from. Other lines are numbered as in the working tree.\n")
	}

	sorted := slices.Clone(comments)
	slices.SortStableFunc(sorted, func(a, b DiffComment) int {
		return cmp.Or(cmp.Compare(a.Path, b.Path), cmp.Compare(a.StartLine, b.StartLine))
	})
	for i, c := range sorted {
		fmt.Fprintf(&b, "\n## %d. %s (%s)\n", i+1, c.Path, diffLineLabel(c))
		writeFencedQuote(&b, c.Quote)
		writeQuoted(&b, strings.TrimSpace(c.Text))
	}
	return b.String()
}

func diffLineLabel(c DiffComment) string {
	removed := func(side DiffSide) string {
		if side == DiffSideDeletions {
			return "removed "
		}
		return ""
	}
	switch {
	case c.StartSide != c.EndSide:
		return fmt.Sprintf("%sline %d to %sline %d", removed(c.StartSide), c.StartLine, removed(c.EndSide), c.EndLine)
	case c.StartLine == c.EndLine:
		return fmt.Sprintf("%sline %d", removed(c.StartSide), c.StartLine)
	default:
		return fmt.Sprintf("%slines %d-%d", removed(c.StartSide), c.StartLine, c.EndLine)
	}
}

// writeFencedQuote fences with more backticks than the code holds, so quoted
// Markdown cannot close the block early.
func writeFencedQuote(b *strings.Builder, quote string) {
	quote = strings.Trim(quote, "\n")
	if strings.TrimSpace(quote) == "" {
		return
	}
	lines := strings.Split(quote, "\n")
	if extra := len(lines) - maxQuotedDiffLines; extra > 0 {
		lines = append(lines[:maxQuotedDiffLines:maxQuotedDiffLines], fmt.Sprintf("... (%d more lines)", extra))
	}
	fence := "```"
	for strings.Contains(quote, fence) {
		fence += "`"
	}
	fmt.Fprintf(b, "%s\n%s\n%s\n", fence, strings.Join(lines, "\n"), fence)
}
