package tui

const unknownViewType = "unknown"

// ViewType represents which view is active.
type ViewType int

const (
	ViewSessions ViewType = iota
	ViewMessages
	ViewReview
)

// String returns the lowercase name of the view type for scope matching.
func (v ViewType) String() string {
	switch v {
	case ViewSessions:
		return "sessions"
	case ViewMessages:
		return "messages"
	case ViewReview:
		return "review"
	default:
		return unknownViewType
	}
}
