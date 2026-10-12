package webhook

import (
	"encoding/json"
	"strings"
)

// feedItemFields lists the render-critical subset of the canonical item
// contract (docs/decisions/2026-07-24-canonical-item-contract.md): the fields the
// feed UI needs for a first-party-quality row. The remaining contract fields
// (num, author, body, labels, state, updatedAt) are optional enrichment —
// state notably drives lifecycle — and their absence is not a shape warning.
// Advisory only — never enforced at ingress.
var feedItemFields = []string{"id", "kind", "repo", "title", "url"}

// MissingFeedItemFields returns which of the feed-item fields the payload
// lacks (absent, or present but not a non-empty string). A nil/empty return
// means the payload will render like a first-party feed item.
func MissingFeedItemFields(payload []byte) []string {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		return append([]string(nil), feedItemFields...)
	}
	var missing []string
	for _, name := range feedItemFields {
		raw, ok := fields[name]
		if !ok {
			missing = append(missing, name)
			continue
		}
		var s string
		if err := json.Unmarshal(raw, &s); err != nil || strings.TrimSpace(s) == "" {
			missing = append(missing, name)
		}
	}
	return missing
}
