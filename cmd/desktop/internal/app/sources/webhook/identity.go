package webhook

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

// identity derives the observation's stable key and its promoted title/url
// columns from the delivered JSON. A top-level "id" (string or number) is the
// stable identity — re-deliveries with the same id update the same inbox
// item. Without one, the key is the body's content hash, so an exact
// duplicate delivery deduplicates and any changed body is a new item.
// "title" and "url" are promoted when present so the item renders in feeds;
// everything else stays inside the opaque payload.
func identity(path string, body []byte) (key, title, url string) {
	var fields map[string]json.RawMessage
	// Non-object JSON (arrays, scalars) is accepted; it just has no fields
	// to promote.
	_ = json.Unmarshal(body, &fields)

	if raw, ok := fields["id"]; ok {
		var s string
		if err := json.Unmarshal(raw, &s); err == nil && strings.TrimSpace(s) != "" {
			key = strings.TrimSpace(s)
		} else {
			var n json.Number
			if err := json.Unmarshal(raw, &n); err == nil {
				key = n.String()
			}
		}
	}
	if key == "" {
		sum := sha256.Sum256(body)
		key = hex.EncodeToString(sum[:])
	}

	if raw, ok := fields["title"]; ok {
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			title = strings.TrimSpace(s)
		}
	}
	if title == "" {
		short := key
		if len(short) > 12 {
			short = short[:12]
		}
		title = path + " · " + short
	}

	if raw, ok := fields["url"]; ok {
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			s = strings.TrimSpace(s)
			if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
				url = s
			}
		}
	}
	return key, title, url
}

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
