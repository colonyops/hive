package review

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// Feedback formats comments on the document at docPath as the text an agent
// reads. A non-empty note leads, under "General Notes". It returns "" when
// there are no comments and no note.
//
// Format:
//
//	General Notes:
//	<note>
//
//	---
//
//	Document: <path>
//	Comments: <count>
//
//	Lines <start>-<end>:
//	> <context line>
//	<comment>
func Feedback(docPath string, comments []Comment, note string) string {
	if len(comments) == 0 && note == "" {
		return ""
	}

	var b strings.Builder
	if note != "" {
		fmt.Fprintf(&b, "General Notes:\n%s\n\n---\n\n", note)
	}

	fmt.Fprintf(&b, "Document: %s\n", docPath)
	fmt.Fprintf(&b, "Comments: %d\n", len(comments))

	sorted := slices.Clone(comments)
	slices.SortStableFunc(sorted, func(a, b Comment) int { return a.StartLine - b.StartLine })

	for _, c := range sorted {
		b.WriteString("\n")
		if c.StartLine == c.EndLine {
			fmt.Fprintf(&b, "Line %d:\n", c.StartLine)
		} else {
			fmt.Fprintf(&b, "Lines %d-%d:\n", c.StartLine, c.EndLine)
		}
		if c.ContextText != "" {
			for line := range strings.SplitSeq(ansiPattern.ReplaceAllString(c.ContextText, ""), "\n") {
				fmt.Fprintf(&b, "> %s\n", line)
			}
		}
		b.WriteString(c.CommentText)
		b.WriteString("\n")
	}

	return b.String()
}
