package relayformat

import (
	"cmp"
	"mime"
	"net/url"
	"strings"
)

// parseGotify reads a gotify client's POST .../message, sent as JSON or, from
// the gotify CLI, as a form. Like ntfy, every message is its own item.
func parseGotify(d Delivery) Item {
	fields, extras := gotifyFields(d)
	message := fields["message"]

	id := deliveryHash(d)
	payload := map[string]any{
		"id":    id,
		"kind":  "Notification",
		"title": cmp.Or(fields["title"], summary(message), "gotify"),
	}
	setIfPresent(payload, "body", message)
	if n, ok := priority(fields["priority"]); ok {
		payload["priority"] = n
	}
	setIfPresent(payload, "url", httpURL(gotifyClickURL(extras)))
	payload["raw"] = raw(d)
	return Item{Key: id, Payload: payload}
}

func gotifyFields(d Delivery) (map[string]string, map[string]any) {
	if obj, ok := decodeObject(d.Body); ok {
		extras, _ := obj["extras"].(map[string]any)
		return map[string]string{
			"title":    field(obj, "title"),
			"message":  field(obj, "message"),
			"priority": field(obj, "priority"),
		}, extras
	}
	if !isForm(d) {
		return map[string]string{"message": strings.TrimSpace(string(d.Body))}, nil
	}
	form, err := url.ParseQuery(string(d.Body))
	if err != nil {
		return map[string]string{"message": strings.TrimSpace(string(d.Body))}, nil
	}
	return map[string]string{
		"title":    strings.TrimSpace(form.Get("title")),
		"message":  strings.TrimSpace(form.Get("message")),
		"priority": strings.TrimSpace(form.Get("priority")),
	}, nil
}

func isForm(d Delivery) bool {
	ct := cmp.Or(d.ContentType, header(d.Headers, "Content-Type"))
	mediaType, _, err := mime.ParseMediaType(ct)
	return err == nil && mediaType == "application/x-www-form-urlencoded"
}

func gotifyClickURL(extras map[string]any) string {
	notification, _ := extras["client::notification"].(map[string]any)
	click, _ := notification["click"].(map[string]any)
	return field(click, "url")
}

// gotifyBodyHasMessage covers a gotify client that authenticated with
// X-Gotify-Key: the relay drops that header before storing the delivery.
func gotifyBodyHasMessage(d Delivery) bool {
	if obj, ok := decodeObject(d.Body); ok {
		return field(obj, "message") != ""
	}
	if !isForm(d) {
		return false
	}
	form, err := url.ParseQuery(string(d.Body))
	return err == nil && strings.TrimSpace(form.Get("message")) != ""
}
