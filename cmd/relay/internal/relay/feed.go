package relay

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"time"
	"unicode/utf8"
)

type Feed struct {
	store Store
}

func NewFeed(store Store) *Feed {
	return &Feed{store: store}
}

type Page struct {
	Deliveries []DeliveryView `json:"deliveries"`
	Next       string         `json:"next"`
	More       bool           `json:"more"`
}

type DeliveryView struct {
	ID            string              `json:"id"`
	ReceivedAt    time.Time           `json:"received_at"`
	Method        string              `json:"method,omitempty"`
	Path          string              `json:"path,omitempty"`
	Query         url.Values          `json:"query,omitempty"`
	Headers       map[string][]string `json:"headers,omitempty"`
	Body          string              `json:"body,omitempty"`     // text or JSON as-is
	BodyBase64    string              `json:"body_b64,omitempty"` // otherwise
	ContentType   string              `json:"content_type,omitempty"`
	ContentLength int64               `json:"content_length"`
	Enc           string              `json:"enc,omitempty"`  // "age" when sealed
	Blob          string              `json:"blob,omitempty"` // base64 ciphertext when sealed
}

// ListAfter checks the source belongs to email (KindNotFound otherwise), then
// pages the store. Next is the last ID in the page ("" when empty).
func (f *Feed) ListAfter(ctx context.Context, email, sourceID, after string, limit int) (Page, error) {
	if _, err := f.store.Sources().Get(ctx, email, sourceID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return Page{}, NewError(KindNotFound, "source not found")
		}
		return Page{}, fmt.Errorf("look up source: %w", err)
	}
	rows, more, err := f.store.Deliveries().ListAfter(ctx, sourceID, after, limit)
	if err != nil {
		return Page{}, fmt.Errorf("list deliveries: %w", err)
	}
	page := Page{Deliveries: make([]DeliveryView, 0, len(rows)), More: more}
	for _, d := range rows {
		v, err := viewDelivery(d)
		if err != nil {
			return Page{}, err
		}
		page.Deliveries = append(page.Deliveries, v)
	}
	if len(rows) > 0 {
		page.Next = rows[len(rows)-1].ID
	}
	return page, nil
}

func viewDelivery(d Delivery) (DeliveryView, error) {
	v := DeliveryView{ID: d.ID, ReceivedAt: d.ReceivedAt}
	if d.Enc {
		v.ContentLength = int64(len(d.Record))
		v.Enc = "age"
		v.Blob = base64.StdEncoding.EncodeToString(d.Record)
		return v, nil
	}
	var rec Record
	if err := json.Unmarshal(d.Record, &rec); err != nil {
		return DeliveryView{}, fmt.Errorf("decode delivery %s: %w", d.ID, err)
	}
	v.Method = rec.Method
	v.Path = rec.Path
	v.Query = rec.Query
	v.Headers = rec.Headers
	v.ContentType = rec.ContentType
	v.ContentLength = int64(len(rec.Body))
	if utf8.Valid(rec.Body) && bytes.IndexByte(rec.Body, 0) < 0 {
		v.Body = string(rec.Body)
	} else {
		v.BodyBase64 = base64.StdEncoding.EncodeToString(rec.Body)
	}
	return v, nil
}
