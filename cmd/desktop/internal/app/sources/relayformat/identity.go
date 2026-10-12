package relayformat

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

// GenericIdentity is the rule the local webhook listener and the relay share.
// A top-level "id" (string or number) is the stable identity, so re-deliveries
// with the same id update the same inbox item. Without one, the key is the
// body's content hash, so an exact duplicate delivery deduplicates and any
// changed body is a new item. "title" and "url" are promoted when present so
// the item renders in feeds.
func GenericIdentity(path string, body []byte) (key, title, url string) {
	var fields map[string]json.RawMessage
	// Non-object JSON and non-JSON bodies are accepted; they just have no
	// fields to promote.
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
