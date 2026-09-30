package models

import (
	"encoding/json"
	"strings"
)

// CanonicalFields is the one shared decode for the canonical top-level
// id/kind/state strings of an item payload (docs/decisions/0008). Every
// reader of the canonical contract goes through it — the action-item decode
// in dispatch and the webhook classifier in the webhook connector — so the
// string-or-number id rule has exactly one implementation.
//
// A payload that is not a JSON object, or that omits a field or gives it a
// non-string value, yields "" for that field rather than an error: the
// canonical fields are optional by contract.
func CanonicalFields(payload []byte) (id, kind, state string) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		return "", "", ""
	}

	if raw, ok := fields["id"]; ok {
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			id = strings.TrimSpace(s)
		} else {
			var n json.Number
			if err := json.Unmarshal(raw, &n); err == nil {
				id = n.String()
			}
		}
	}
	if raw, ok := fields["kind"]; ok {
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			kind = strings.TrimSpace(s)
		}
	}
	if raw, ok := fields["state"]; ok {
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			state = strings.TrimSpace(s)
		}
	}
	return id, kind, state
}

// FeedFields is the one shared decode for the presentation an item payload
// carries: the title and url a feed row renders, and the time the item's own
// source says it last changed.
//
// Both boundaries that mint an inbox row read it — the poll producer, for a
// message a source emitted, and CommitBatch, for a key a function node
// synthesized (ADR function-node-per-entity-feed-items) — so `updatedAt` on a
// per-entity payload means what it means on a source's own.
//
// updatedAt is unix milliseconds. Zero means the payload did not say, and the
// caller substitutes its own clock rather than stamping the item at the epoch.
func FeedFields(payload []byte) (title, url string, updatedAt int64) {
	var wire struct {
		Title     string `json:"title"`
		URL       string `json:"url"`
		UpdatedAt int64  `json:"updatedAt"`
	}
	_ = json.Unmarshal(payload, &wire)
	return wire.Title, wire.URL, wire.UpdatedAt
}
