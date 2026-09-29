package review

// Modal interface for modal dialogs.
// This is a minimal interface for common modal operations.
type Modal interface {
	View() string
	Cancelled() bool
}

// Compile-time checks to ensure modals implement the Modal interface.
var (
	_ Modal = (*CommentModal)(nil)
	_ Modal = (*FinalizationModal)(nil)
)
