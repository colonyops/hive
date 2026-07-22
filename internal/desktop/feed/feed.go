// Package feed is the desktop's GitHub fetch layer: the wire Item type and a
// cached, coalesced LiveProvider the pipeline producer uses to turn a source
// into event_log rows. The old profiles/feeds config machinery folded into
// the flow graph (internal/desktop/pipeline/flow); what remains here is the
// GitHub fetch core.
package feed

// Item is a normalized GitHub item (PR or issue) produced by a source fetch.
// Its JSON payload is stored in a durable inbox row.
type Item struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"` // "PR" | "Issue"
	Repo      string `json:"repo"`
	Num       int    `json:"num"`
	Title     string `json:"title"`
	Author    string `json:"author"`
	State     string `json:"state,omitempty"`
	UpdatedAt int64  `json:"updatedAt"` // GitHub's last-updated time, unix milliseconds
	Unread    bool   `json:"unread"`
	// Reason is the GitHub notification reason (e.g. "review_requested"),
	// empty for items known only from search.
	Reason string   `json:"reason,omitempty"`
	Labels []string `json:"labels"`
	Branch string   `json:"branch"`
	Body   string   `json:"body"`
	Prompt string   `json:"prompt"`
	URL    string   `json:"url"`
}
