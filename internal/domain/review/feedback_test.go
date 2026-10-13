package review

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
			comments: []Comment{{StartLine: 5, EndLine: 5, ContextText: "This is the context", CommentText: "This needs improvement"}},
			want:     "Document: plans/test.md\nComments: 1\n\nLine 5:\n> This is the context\nThis needs improvement\n",
		},
		{
			name: "multiple comments sorted by line",
			comments: []Comment{
				{StartLine: 15, EndLine: 17, ContextText: "Second context", CommentText: "Second feedback"},
				{StartLine: 5, EndLine: 5, ContextText: "First context", CommentText: "First feedback"},
			},
			want: "Document: plans/test.md\nComments: 2\n\nLine 5:\n> First context\nFirst feedback\n\nLines 15-17:\n> Second context\nSecond feedback\n",
		},
		{
			name:     "multiline context",
			comments: []Comment{{StartLine: 10, EndLine: 12, ContextText: "Line 1\nLine 2\nLine 3", CommentText: "Check these lines"}},
			want:     "Document: plans/test.md\nComments: 1\n\nLines 10-12:\n> Line 1\n> Line 2\n> Line 3\nCheck these lines\n",
		},
		{
			name:     "ansi codes stripped from context",
			comments: []Comment{{StartLine: 1, EndLine: 1, ContextText: "\x1b[1mbold\x1b[0m", CommentText: "ok"}},
			want:     "Document: plans/test.md\nComments: 1\n\nLine 1:\n> bold\nok\n",
		},
		{
			name:     "empty context",
			comments: []Comment{{StartLine: 2, EndLine: 3, CommentText: "no quote"}},
			want:     "Document: plans/test.md\nComments: 1\n\nLines 2-3:\nno quote\n",
		},
		{
			name:     "note with comments",
			comments: []Comment{{StartLine: 5, EndLine: 5, ContextText: "ctx", CommentText: "fix"}},
			note:     "Overall fine",
			want:     "General Notes:\nOverall fine\n\n---\n\nDocument: plans/test.md\nComments: 1\n\nLine 5:\n> ctx\nfix\n",
		},
		{
			name: "note only",
			note: "Rethink the approach",
			want: "General Notes:\nRethink the approach\n\n---\n\nDocument: plans/test.md\nComments: 0\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Feedback("plans/test.md", tt.comments, tt.note))
		})
	}
}
