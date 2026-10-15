package review

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const header = "# Document Feedback\n\nDocument: plans/test.md\n\n"

func TestFeedback(t *testing.T) {
	tests := []struct {
		name     string
		comments []Comment
		note     string
		want     string
	}{
		{
			name: "no comments and no note",
			want: "",
		},
		{
			name:     "single comment",
			comments: []Comment{{StartLine: 5, EndLine: 5, ContextText: "proposal", CommentText: "Call it an RFC"}},
			want: header + "I've reviewed this document and have 1 piece of feedback:\n\n" +
				"## 1. (line 5) Feedback on: \"proposal\"\n> Call it an RFC\n",
		},
		{
			name: "comments sorted by line, note first",
			comments: []Comment{
				{StartLine: 15, EndLine: 17, ContextText: "Second", CommentText: "Second feedback"},
				{StartLine: 5, EndLine: 5, ContextText: "First", CommentText: "First feedback"},
			},
			note: "Overall fine",
			want: header + "I've reviewed this document and have 3 pieces of feedback:\n\n" +
				"## 1. General feedback\n> Overall fine\n\n" +
				"## 2. (line 5) Feedback on: \"First\"\n> First feedback\n\n" +
				"## 3. (lines 15-17) Feedback on: \"Second\"\n> Second feedback\n",
		},
		{
			name:     "multi-line quote and comment",
			comments: []Comment{{StartLine: 10, EndLine: 12, ContextText: "Line 1\nLine 2", CommentText: "Check\n\nthese"}},
			want: header + "I've reviewed this document and have 1 piece of feedback:\n\n" +
				"## 1. (lines 10-12) Feedback on: \"Line 1\\nLine 2\"\n> Check\n>\n> these\n",
		},
		{
			name:     "ansi codes stripped from the quote",
			comments: []Comment{{StartLine: 1, EndLine: 1, ContextText: "\x1b[1mbold\x1b[0m", CommentText: "ok"}},
			want: header + "I've reviewed this document and have 1 piece of feedback:\n\n" +
				"## 1. (line 1) Feedback on: \"bold\"\n> ok\n",
		},
		{
			name:     "no quote",
			comments: []Comment{{StartLine: 2, EndLine: 3, CommentText: "no quote"}},
			want: header + "I've reviewed this document and have 1 piece of feedback:\n\n" +
				"## 1. (lines 2-3)\n> no quote\n",
		},
		{
			name: "note only",
			note: "Rethink the approach",
			want: header + "I've reviewed this document and have 1 piece of feedback:\n\n" +
				"## 1. General feedback\n> Rethink the approach\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Feedback("plans/test.md", tt.comments, tt.note))
		})
	}
}
