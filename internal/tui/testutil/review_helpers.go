package testutil

import (
	"testing"
	"time"

	"github.com/charmbracelet/x/exp/golden"

	corereview "github.com/hay-kot/hive/internal/core/review"
	"github.com/hay-kot/hive/internal/core/terminal"
)

// RequireGolden compares output with a golden file using golden.RequireEqual().
func RequireGolden(t *testing.T, output string) {
	t.Helper()
	golden.RequireEqual(t, []byte(output))
}

// StripANSI removes ANSI escape codes from content.
// Re-exports terminal.StripANSI for convenience.
func StripANSI(content string) string {
	return terminal.StripANSI(content)
}

// CreateTestComment creates a test comment with a fixed timestamp.
func CreateTestComment(line int, text string) corereview.Comment {
	return corereview.Comment{
		ID:          "test-comment-id",
		SessionID:   "test-session-id",
		StartLine:   line,
		EndLine:     line,
		ContextText: "Test context",
		CommentText: text,
		CreatedAt:   time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
	}
}
