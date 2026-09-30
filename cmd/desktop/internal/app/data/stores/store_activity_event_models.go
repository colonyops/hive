package stores

// Category and Severity remain strings because the activity service owns
// their enums.
type ActivityEvent struct {
	ID        int64             `json:"id"`
	CreatedAt int64             `json:"createdAt"`
	Category  string            `json:"category"`
	Severity  string            `json:"severity"`
	Title     string            `json:"title"`
	Body      string            `json:"body"`
	Source    string            `json:"source"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type ActivityEventCreate struct {
	Category string
	Severity string
	Title    string
	Body     string
	Source   string
	Metadata map[string]string
}
