package review

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// Feedback formats comments on the document at docPath as the text an agent
// reads: a heading, the document, a count, then one numbered entry each, in
// document order, with a non-empty note leading as general feedback. It
// returns "" when there are no comments and no note.
//
// Format:
//
//	# Document Feedback
//
//	Document: <path>
//
//	I've reviewed this document and have <n> pieces of feedback:
//
//	## 1. General feedback
//	> <note>
//
//	## 2. (lines <start>-<end>) Feedback on: "<quote>"
//	> <comment>
func Feedback(docPath string, comments []Comment, note string) string {
	note = strings.TrimSpace(note)
	count := len(comments)
	if note != "" {
		count++
	}
	if count == 0 {
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Document Feedback\n\nDocument: %s\n\n", docPath)
	if count == 1 {
		b.WriteString("I've reviewed this document and have 1 piece of feedback:\n")
	} else {
		fmt.Fprintf(&b, "I've reviewed this document and have %d pieces of feedback:\n", count)
	}

	n := 0
	entry := func(heading string) {
		n++
		fmt.Fprintf(&b, "\n## %d. %s\n", n, heading)
	}

	if note != "" {
		entry("General feedback")
		writeQuoted(&b, note)
	}

	sorted := slices.Clone(comments)
	slices.SortStableFunc(sorted, func(a, b Comment) int { return a.StartLine - b.StartLine })
	for _, c := range sorted {
		quote := strings.TrimSpace(ansiPattern.ReplaceAllString(c.ContextText, ""))
		text := strings.TrimSpace(c.CommentText)
		if quote == "" {
			entry(lineLabel(c))
		} else {
			entry(fmt.Sprintf("%s Feedback on: %q", lineLabel(c), quote))
		}
		writeQuoted(&b, text)
	}

	return b.String()
}

func lineLabel(c Comment) string {
	if c.StartLine == c.EndLine {
		return fmt.Sprintf("(line %d)", c.StartLine)
	}
	return fmt.Sprintf("(lines %d-%d)", c.StartLine, c.EndLine)
}

func writeQuoted(b *strings.Builder, text string) {
	for line := range strings.SplitSeq(text, "\n") {
		if line == "" {
			b.WriteString(">\n")
			continue
		}
		fmt.Fprintf(b, "> %s\n", line)
	}
}
