package review

import "time"

// Session represents an active review session for a document.
type Session struct {
	ID           string
	DocumentPath string
	ContentHash  string // SHA256 hash of document content
	CreatedAt    time.Time
	FinalizedAt  *time.Time // nil if not finalized
	// Diff-specific fields
	SessionName string // Human-readable name for diff sessions (e.g., "feat-auth-vs-main")
	DiffContext string // Git context for diff sessions (e.g., "main..feat-auth", "staged", "unstaged")
}

// Comment represents inline feedback on a document section.
type Comment struct {
	ID          string
	SessionID   string
	StartLine   int
	EndLine     int
	ContextText string
	CommentText string
	CreatedAt   time.Time
	// Diff-specific fields
	Side string // For diff comments: "old" (deletions, "-" lines) or "new" (additions, "+" lines). Empty for document reviews.
}

// IsFinalized returns true if the review session has been finalized.
func (s Session) IsFinalized() bool {
	return s.FinalizedAt != nil
}
