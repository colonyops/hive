package relay

import (
	"context"
	"errors"
	"time"

	"filippo.io/age"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/relayformat"
)

const (
	previewPageSize     = 500
	defaultPreviewCount = 20
)

// RecentDelivery is one delivery as the node editor previews it: the request,
// the dialect auto matched, and the item it parsed to. Error is set instead of
// Item when the delivery could not be parsed.
type RecentDelivery struct {
	ID          string         `json:"id"`
	ReceivedAt  time.Time      `json:"receivedAt"`
	Method      string         `json:"method"`
	Path        string         `json:"path"`
	ContentType string         `json:"contentType"`
	Body        string         `json:"body"`
	Dialect     string         `json:"dialect"`
	Item        map[string]any `json:"item"`
	Key         string         `json:"key"`
	Error       string         `json:"error"`
}

// RecentDeliveries returns the newest limit deliveries, oldest first. The log
// is read forward from the start because the relay pages only by cursor; the
// per-source cap keeps that to a couple of pages.
func (c *Client) RecentDeliveries(ctx context.Context, sourceID string, limit int, identity *age.X25519Identity) ([]RecentDelivery, error) {
	if limit <= 0 {
		limit = defaultPreviewCount
	}
	var tail []DeliveryView
	after := ""
	for {
		page, err := c.Deliveries(ctx, sourceID, after, previewPageSize)
		if err != nil {
			return nil, err
		}
		tail = append(tail, page.Deliveries...)
		if len(tail) > limit {
			tail = tail[len(tail)-limit:]
		}
		if page.Next != "" {
			after = page.Next
		}
		if !page.More || len(page.Deliveries) == 0 {
			break
		}
	}
	out := make([]RecentDelivery, 0, len(tail))
	for _, view := range tail {
		out = append(out, preview(identity, view))
	}
	return out, nil
}

func preview(identity *age.X25519Identity, view DeliveryView) RecentDelivery {
	rd := RecentDelivery{
		ID:          view.ID,
		ReceivedAt:  view.ReceivedAt,
		Method:      view.Method,
		Path:        view.Path,
		ContentType: view.ContentType,
	}
	d, err := decode(identity, view)
	if errors.Is(err, errUnopenable) {
		rd.Error = "this delivery is encrypted and this device holds no key that opens it"
		return rd
	}
	if err != nil {
		rd.Error = err.Error()
		return rd
	}
	rd.Method, rd.Path, rd.ContentType = d.Method, d.Path, d.ContentType
	rd.Body = string(d.Body)
	items, dialect, err := relayformat.Parse(relayformat.DialectAuto, d)
	if err != nil {
		rd.Error = err.Error()
		return rd
	}
	rd.Dialect = dialect
	if len(items) > 0 {
		rd.Key = items[0].Key
		rd.Item = items[0].Payload
	}
	return rd
}
