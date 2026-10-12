package relayformat

import (
	"cmp"
	"strings"
)

var (
	ntfyTitleHeaders    = []string{"X-Title", "Title", "t"}
	ntfyMessageHeaders  = []string{"X-Message", "Message", "m"}
	ntfyPriorityHeaders = []string{"X-Priority", "Priority", "prio", "p"}
	ntfyTagsHeaders     = []string{"X-Tags", "Tags", "tag", "ta"}
	ntfyClickHeaders    = []string{"X-Click", "Click"}
)

// parseNtfy reads an ntfy publish. Each field takes the first value found in
// headers, then the query, then a JSON object body; a body that is not a JSON
// object is the message itself. Every publish is its own item: without an
// explicit message id the key is the delivery id's hash, so two identical
// notifications never collapse into one.
func parseNtfy(d Delivery) Item {
	obj, isObject := decodeObject(d.Body)
	text := ""
	if !isObject {
		text = strings.TrimSpace(string(d.Body))
	}

	topic := ""
	if segs := pathSegments(d.Path); len(segs) > 0 {
		topic = segs[0]
	}
	topic = cmp.Or(topic, query(d.Query, "topic"), field(obj, "topic"))

	title := cmp.Or(header(d.Headers, ntfyTitleHeaders...), query(d.Query, "title", "t"), field(obj, "title"))
	message := cmp.Or(header(d.Headers, ntfyMessageHeaders...), query(d.Query, "message", "m"), field(obj, "message"), text)
	prio := cmp.Or(header(d.Headers, ntfyPriorityHeaders...), query(d.Query, "priority", "prio", "p"), field(obj, "priority"))
	click := cmp.Or(header(d.Headers, ntfyClickHeaders...), query(d.Query, "click"), field(obj, "click"))

	tags := splitTags(cmp.Or(header(d.Headers, ntfyTagsHeaders...), query(d.Query, "tags", "tag", "ta")))
	if tags == nil {
		tags = jsonTags(obj["tags"])
	}

	id := cmp.Or(header(d.Headers, "X-Message-ID"), field(obj, "id"), deliveryHash(d))

	payload := map[string]any{
		"id":    id,
		"kind":  "Notification",
		"title": cmp.Or(title, summary(message), topic, "ntfy"),
	}
	setIfPresent(payload, "repo", topic)
	setIfPresent(payload, "body", message)
	setIfPresent(payload, "url", httpURL(click))
	if len(tags) > 0 {
		payload["labels"] = tags
	}
	if n, ok := priority(prio); ok {
		payload["priority"] = n
	}
	payload["raw"] = raw(d)
	return Item{Key: id, Payload: payload}
}

func splitTags(s string) []any {
	var out []any
	for tag := range strings.SplitSeq(s, ",") {
		if tag = strings.TrimSpace(tag); tag != "" {
			out = append(out, tag)
		}
	}
	return out
}

func jsonTags(v any) []any {
	switch v := v.(type) {
	case string:
		return splitTags(v)
	case []any:
		var out []any
		for _, tag := range v {
			if s, ok := tag.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	default:
		return nil
	}
}
