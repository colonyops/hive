package review

import "time"

// Session represents an active review session for a document.
type Session struct {
	ID           string
	DocumentPath string
	ContentHash  string // SHA256 hash of document content
	CreatedAt    time.Time
	FinalizedAt  *time.Time // nil if not finalized
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
}

// IsFinalized returns true if the review session has been finalized.
func (s Session) IsFinalized() bool {
	return s.FinalizedAt != nil
}
