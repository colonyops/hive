package tui

import (
	"fmt"
	"sort"
	"strings"
)

// GenerateReviewFeedback creates a formatted review feedback string from a session.
// Format:
//
//	Document: <path>
//	Comments: <count>
//
//	Lines <start>-<end>:
//	> <context line 1>
//	> <context line 2>
//	<feedback text>
//
//	Lines <start>-<end>:
//	> <context>
//	<feedback>
func GenerateReviewFeedback(session *ReviewSession, docRelPath string) string {
	if session == nil || len(session.Comments) == 0 {
		return ""
	}

	var b strings.Builder

	// Header
	b.WriteString(fmt.Sprintf("Document: %s\n", docRelPath))
	b.WriteString(fmt.Sprintf("Comments: %d\n\n", len(session.Comments)))

	// Sort comments by line number
	sortedComments := make([]ReviewComment, len(session.Comments))
	copy(sortedComments, session.Comments)
	sort.Slice(sortedComments, func(i, j int) bool {
		return sortedComments[i].StartLine < sortedComments[j].StartLine
	})

	// Format each comment
	for i, comment := range sortedComments {
		if i > 0 {
			b.WriteString("\n")
		}

		// Line range
		if comment.StartLine == comment.EndLine {
			b.WriteString(fmt.Sprintf("Line %d:\n", comment.StartLine))
		} else {
			b.WriteString(fmt.Sprintf("Lines %d-%d:\n", comment.StartLine, comment.EndLine))
		}

		// Context (quoted)
		if comment.ContextText != "" {
			for line := range strings.SplitSeq(comment.ContextText, "\n") {
				b.WriteString(fmt.Sprintf("> %s\n", line))
			}
		}

		// Feedback
		b.WriteString(comment.CommentText)
		b.WriteString("\n")
	}

	return b.String()
}
