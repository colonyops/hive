package relay_test

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/relay/internal/relay"
	"github.com/colonyops/hive/pkg/randid"
)

func TestFeedBodies(t *testing.T) {
	tests := []struct {
		name     string
		body     []byte
		wantText string
		wantB64  string
	}{
		{name: "json", body: []byte(`{"a":1}`), wantText: `{"a":1}`},
		{name: "text", body: []byte("héllo"), wantText: "héllo"},
		{name: "invalid utf8", body: []byte{0xff, 0x01}, wantB64: base64.StdEncoding.EncodeToString([]byte{0xff, 0x01})},
		{name: "nul", body: []byte("a\x00b"), wantB64: base64.StdEncoding.EncodeToString([]byte("a\x00b"))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newStore(t)
			src, token := newSource(t, store, alice, "")
			_, err := relay.NewIngress(store, relay.DefaultLimits()).Accept(t.Context(), token, "", relay.Record{
				Method:      "POST",
				Path:        "/p",
				ContentType: "application/octet-stream",
				Body:        tt.body,
			})
			require.NoError(t, err)

			page, err := relay.NewFeed(store).ListAfter(t.Context(), alice, src.ID, "", 10)
			require.NoError(t, err)
			require.Len(t, page.Deliveries, 1)
			d := page.Deliveries[0]
			assert.Equal(t, tt.wantText, d.Body)
			assert.Equal(t, tt.wantB64, d.BodyBase64)
			assert.Equal(t, int64(len(tt.body)), d.ContentLength)
			assert.Equal(t, "POST", d.Method)
			assert.Equal(t, "/p", d.Path)
			assert.Equal(t, "application/octet-stream", d.ContentType)
		})
	}
}

func TestFeedPaging(t *testing.T) {
	store := newStore(t)
	src, token := newSource(t, store, alice, "")
	ingress := relay.NewIngress(store, relay.DefaultLimits())
	var ids []string
	for range 5 {
		id, err := ingress.Accept(t.Context(), token, "", relay.Record{Method: "POST"})
		require.NoError(t, err)
		ids = append(ids, id)
	}
	feed := relay.NewFeed(store)

	var (
		got   []string
		after string
		pages int
	)
	for {
		page, err := feed.ListAfter(t.Context(), alice, src.ID, after, 2)
		require.NoError(t, err)
		pages++
		for _, d := range page.Deliveries {
			got = append(got, d.ID)
		}
		if !page.More {
			assert.Equal(t, ids[len(ids)-1], page.Next)
			break
		}
		after = page.Next
	}
	assert.Equal(t, ids, got)
	assert.Equal(t, 3, pages)

	page, err := feed.ListAfter(t.Context(), alice, src.ID, ids[4], 2)
	require.NoError(t, err)
	assert.Empty(t, page.Deliveries)
	assert.Empty(t, page.Next)
	assert.False(t, page.More)
}

func TestFeedOtherAccount(t *testing.T) {
	store := newStore(t)
	src, _ := newSource(t, store, alice, "")
	_, err := relay.NewFeed(store).ListAfter(t.Context(), bob, src.ID, "", 10)
	requireKind(t, relay.KindNotFound, err)
}

func TestFeedSealed(t *testing.T) {
	store := newStore(t)
	src, _ := newSource(t, store, alice, "")
	blob := []byte("age-encryption.org/v1 ciphertext")
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.Deliveries().Append(t.Context(), alice, relay.Delivery{
		ID:         randid.UUIDv7(),
		SourceID:   src.ID,
		ReceivedAt: at,
		Enc:        true,
		Size:       int64(len(blob)),
		Record:     blob,
	}, 1<<20, 10))

	page, err := relay.NewFeed(store).ListAfter(t.Context(), alice, src.ID, "", 10)
	require.NoError(t, err)
	require.Len(t, page.Deliveries, 1)
	assert.Equal(t, relay.DeliveryView{
		ID:            page.Deliveries[0].ID,
		ReceivedAt:    at,
		ContentLength: int64(len(blob)),
		Enc:           "age",
		Blob:          base64.StdEncoding.EncodeToString(blob),
	}, page.Deliveries[0])
}
