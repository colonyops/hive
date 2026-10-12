// Package relayformat turns one stored relay delivery into canonical items.
// It is a leaf over the standard library so the webhook listener and the relay
// connector can share the generic identity rule without importing each other.
package relayformat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	DialectAuto         = "auto"
	DialectGeneric      = "generic"
	DialectNtfy         = "ntfy"
	DialectGotify       = "gotify"
	DialectHealthchecks = "healthchecks"
)

// DefaultKind matches the label the feed gives a webhook item, so a relay
// item and a local webhook item read the same.
const DefaultKind = "Webhook"

// Delivery is one stored HTTP request, decoded from the relay's wire shape.
type Delivery struct {
	ID          string
	ReceivedAt  time.Time
	Method      string
	Path        string
	Query       url.Values
	Headers     map[string][]string
	Body        []byte
	ContentType string
}

// Item is one canonical item a delivery yields. Payload holds the canonical
// fields plus "raw": {method, path, query, headers, body}, with body decoded
// when it is JSON so a JSON Pointer can reach into it.
type Item struct {
	Key     string
	Payload map[string]any
}

func Dialects() []string {
	return []string{DialectAuto, DialectGeneric, DialectNtfy, DialectGotify, DialectHealthchecks}
}

// Parse returns the items a delivery yields under dialect, and the dialect that
// parsed it: under "auto", the one the detector picked.
func Parse(dialect string, d Delivery) ([]Item, string, error) {
	if dialect == DialectAuto {
		dialect = detect(d)
	}
	switch dialect {
	case DialectGeneric:
		return []Item{parseGeneric(d)}, DialectGeneric, nil
	case DialectNtfy:
		return []Item{parseNtfy(d)}, DialectNtfy, nil
	case DialectGotify:
		return []Item{parseGotify(d)}, DialectGotify, nil
	case DialectHealthchecks:
		// A delivery to the ingress root names no check, so it falls back to
		// generic rather than failing the tick on one stray request.
		if item, ok := parseHealthchecks(d); ok {
			return []Item{item}, DialectHealthchecks, nil
		}
		return []Item{parseGeneric(d)}, DialectGeneric, nil
	default:
		return nil, "", fmt.Errorf("unknown dialect %q (want one of %s)", dialect, strings.Join(Dialects(), ", "))
	}
}

func parseGeneric(d Delivery) Item {
	path := d.Path
	if path == "" {
		path = "/"
	}
	key, title, link := GenericIdentity(path, d.Body)

	payload, ok := decodeObject(d.Body)
	if !ok {
		payload = map[string]any{"body": string(d.Body)}
	}
	if !hasValue(payload["id"]) {
		payload["id"] = key
	}
	if !hasValue(payload["title"]) {
		payload["title"] = title
	}
	if link != "" && !hasValue(payload["url"]) {
		payload["url"] = link
	}
	if !hasValue(payload["kind"]) {
		payload["kind"] = DefaultKind
	}
	payload["raw"] = raw(d)
	return Item{Key: key, Payload: payload}
}

func raw(d Delivery) map[string]any {
	var body any = string(d.Body)
	if v, ok := decodeAny(d.Body); ok {
		body = v
	}
	return map[string]any{
		"method":  d.Method,
		"path":    d.Path,
		"query":   multiMap(d.Query),
		"headers": multiMap(d.Headers),
		"body":    body,
	}
}

// multiMap converts to the generic JSON types so a pointer resolves over raw
// the same way whether it was built here or decoded from JSON.
func multiMap(m map[string][]string) map[string]any {
	out := make(map[string]any, len(m))
	for k, vs := range m {
		list := make([]any, len(vs))
		for i, v := range vs {
			list[i] = v
		}
		out[k] = list
	}
	return out
}

// decodeAny keeps numbers as json.Number so a large numeric id survives intact.
func decodeAny(body []byte) (any, bool) {
	if !json.Valid(body) {
		return nil, false
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, false
	}
	return v, true
}

func decodeObject(body []byte) (map[string]any, bool) {
	v, ok := decodeAny(body)
	if !ok {
		return nil, false
	}
	obj, ok := v.(map[string]any)
	return obj, ok
}

func hasValue(v any) bool {
	switch v := v.(type) {
	case string:
		return strings.TrimSpace(v) != ""
	case nil:
		return false
	default:
		return true
	}
}
