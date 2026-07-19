// Package feed is the desktop's GitHub fetch layer: the wire Item type and a
// cached, coalesced LiveProvider the pipeline producer uses to turn a source
// into event_log rows. The old profiles/feeds config machinery folded into
// the flow graph (internal/desktop/pipeline/flow); what remains here is the
// GitHub fetch core plus the static action catalog the detail pane still uses.
package feed

// Item is a normalized GitHub item (PR or issue) produced by a source fetch.
// It is JSON-encoded as a source node's event_log payload and re-surfaced as
// a feed_item, so the frontend and the persisted feed both read this shape.
type Item struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"` // "PR" | "Issue"
	Repo   string `json:"repo"`
	Num    int    `json:"num"`
	Title  string `json:"title"`
	Author string `json:"author"`
	Age    string `json:"age"`
	Unread bool   `json:"unread"`
	// Reason is the GitHub notification reason (e.g. "review_requested"),
	// empty for items known only from search.
	Reason string   `json:"reason,omitempty"`
	Labels []string `json:"labels"`
	Branch string   `json:"branch"`
	Body   string   `json:"body"`
	Prompt string   `json:"prompt"`
	URL    string   `json:"url"`
}

// Action is an action offered for a feed item's kind, for the detail-pane
// picker.
type Action struct {
	ID      string `json:"id"`
	Icon    string `json:"icon"`
	Color   string `json:"color"`
	Title   string `json:"title"`
	Sub     string `json:"sub"`
	Primary bool   `json:"primary"`
}

// ActionsFor returns the actions available for a PR or issue. Kind-scoped, not
// data-scoped, until the config-driven (actions.yml) actions editor exists.
func ActionsFor(kind string) []Action {
	if kind == "PR" {
		return []Action{
			{ID: "review-pr", Icon: "play", Color: "#34d399", Title: "Review PR", Sub: "sonnet · read the diff, leave inline comments", Primary: true},
			{ID: "rethink-approach", Icon: "rotate-ccw", Color: "#a78bfa", Title: "Rethink approach", Sub: "opus · re-evaluate design, propose an alternative"},
			{ID: "summarize-changes", Icon: "list", Color: "#60a5fa", Title: "Summarize changes", Sub: "haiku · post a plain-language summary comment"},
			{ID: "reproduce-and-fix", Icon: "sparkles", Color: "#fb7185", Title: "Reproduce & fix", Sub: "sonnet · check out branch, reproduce, patch"},
		}
	}

	return []Action{
		{ID: "start-implementation", Icon: "play", Color: "#34d399", Title: "Start implementation", Sub: "sonnet · new branch, implement from the issue", Primary: true},
		{ID: "investigate", Icon: "search", Color: "#60a5fa", Title: "Investigate", Sub: "sonnet · research and write a findings doc"},
		{ID: "rethink-spec", Icon: "rotate-ccw", Color: "#a78bfa", Title: "Rethink / spec", Sub: "opus · draft a plan before writing code"},
		{ID: "create-hc-task", Icon: "diamond", Color: "#f59e0b", Title: "Create hc task", Sub: "turn this into a Honeycomb epic"},
	}
}
