package review

import (
	"strings"
	"testing"
	"time"
)

func TestGenerateReviewFeedback(t *testing.T) {
	tests := []struct {
		name       string
		session    *Session
		docRelPath string
		want       string
		wantEmpty  bool
	}{
		{
			name:       "nil session",
			session:    nil,
			docRelPath: "plans/test.md",
			wantEmpty:  true,
		},
		{
			name: "empty comments",
			session: &Session{
				ID:       "session-1",
				DocPath:  "/path/to/doc.md",
				Comments: []Comment{},
			},
			docRelPath: "plans/test.md",
			wantEmpty:  true,
		},
		{
			name: "single comment",
			session: &Session{
				ID:      "session-1",
				DocPath: "/path/to/doc.md",
				Comments: []Comment{
					{
						ID:          "comment-1",
						SessionID:   "session-1",
						StartLine:   5,
						EndLine:     5,
						ContextText: "This is the context",
						CommentText: "This needs improvement",
						CreatedAt:   time.Now(),
					},
				},
			},
			docRelPath: "plans/test.md",
			want:       "Document: plans/test.md\nComments: 1\n\nLine 5:\n> This is the context\nThis needs improvement\n",
		},
		{
			name: "multiple comments sorted by line",
			session: &Session{
				ID:      "session-1",
				DocPath: "/path/to/doc.md",
				Comments: []Comment{
					{
						ID:          "comment-2",
						SessionID:   "session-1",
						StartLine:   15,
						EndLine:     17,
						ContextText: "Second context",
						CommentText: "Second feedback",
						CreatedAt:   time.Now(),
					},
					{
						ID:          "comment-1",
						SessionID:   "session-1",
						StartLine:   5,
						EndLine:     5,
						ContextText: "First context",
						CommentText: "First feedback",
						CreatedAt:   time.Now(),
					},
				},
			},
			docRelPath: "plans/test.md",
			want:       "Document: plans/test.md\nComments: 2\n\nLine 5:\n> First context\nFirst feedback\n\nLines 15-17:\n> Second context\nSecond feedback\n",
		},
		{
			name: "multiline context",
			session: &Session{
				ID:      "session-1",
				DocPath: "/path/to/doc.md",
				Comments: []Comment{
					{
						ID:          "comment-1",
						SessionID:   "session-1",
						StartLine:   10,
						EndLine:     12,
						ContextText: "Line 1\nLine 2\nLine 3",
						CommentText: "Check these lines",
						CreatedAt:   time.Now(),
					},
				},
			},
			docRelPath: "research/doc.md",
			want:       "Document: research/doc.md\nComments: 1\n\nLines 10-12:\n> Line 1\n> Line 2\n> Line 3\nCheck these lines\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateReviewFeedback(tt.session, tt.docRelPath)

			if tt.wantEmpty {
				if got != "" {
					t.Errorf("GenerateReviewFeedback() = %q, want empty string", got)
				}
				return
			}

			if got != tt.want {
				t.Errorf("GenerateReviewFeedback() mismatch:\ngot:\n%s\n\nwant:\n%s", got, tt.want)
				// Show detailed diff
				gotLines := strings.Split(got, "\n")
				wantLines := strings.Split(tt.want, "\n")
				maxLen := max(len(gotLines), len(wantLines))
				for i := range maxLen {
					var gotLine, wantLine string
					if i < len(gotLines) {
						gotLine = gotLines[i]
					}
					if i < len(wantLines) {
						wantLine = wantLines[i]
					}
					if gotLine != wantLine {
						t.Logf("Line %d differs:\n  got:  %q\n  want: %q", i+1, gotLine, wantLine)
					}
				}
			}
		})
	}
}
