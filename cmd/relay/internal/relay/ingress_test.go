package relay_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/relay/internal/relay"
)

func TestIngressStoresAsDelivered(t *testing.T) {
	tests := []struct {
		name string
		body []byte
	}{
		{"truncated json", []byte(`{"a":`)},
		{"invalid utf8", []byte{0xff, 0xfe, 'a', 0x00, 0xc3}},
		{"empty", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newStore()
			src, token := newSource(t, store, alice, "")
			ingress := relay.NewIngress(store, relay.DefaultLimits())
			rec := relay.Record{
				Method:      "PUT",
				Path:        "/x/y",
				Query:       url.Values{"q": {"1"}},
				Headers:     map[string][]string{"X-A": {"b"}},
				Body:        tt.body,
				ContentType: "application/json",
				Remote:      "203.0.113.9",
			}
			id, err := ingress.Accept(t.Context(), token, "", rec)
			require.NoError(t, err)

			rows, _, err := store.Deliveries().ListAfter(t.Context(), src.ID, "", 10)
			require.NoError(t, err)
			require.Len(t, rows, 1)
			assert.Equal(t, id, rows[0].ID)
			assert.Equal(t, int64(len(rows[0].Record)), rows[0].Size)

			var got relay.Record
			require.NoError(t, json.Unmarshal(rows[0].Record, &got))
			assert.Equal(t, tt.body, got.Body)
			assert.Equal(t, rec.Method, got.Method)
			assert.Equal(t, rec.Path, got.Path)
			assert.Equal(t, rec.Query, got.Query)
			assert.Equal(t, rec.Remote, got.Remote)

			acct, err := store.Accounts().Get(t.Context(), alice)
			require.NoError(t, err)
			assert.Equal(t, rows[0].Size, acct.BytesUsed)
		})
	}
}

func TestAcceptSourceGoneIsNotFound(t *testing.T) {
	store := newStore()
	src, _ := newSource(t, store, alice, "")
	require.NoError(t, store.Sources().Delete(t.Context(), alice, src.ID))

	_, err := relay.NewIngress(store, relay.DefaultLimits()).AcceptSource(t.Context(), src, relay.Record{Method: "POST"})
	requireKind(t, relay.KindNotFound, err)
}

func TestIngressRejects(t *testing.T) {
	tests := []struct {
		name      string
		secret    string
		token     func(real string) string
		presented string
		want      relay.Kind
	}{
		{name: "unknown token", token: func(string) string { return "nope" }, want: relay.KindNotFound},
		{name: "empty token", token: func(string) string { return "" }, want: relay.KindNotFound},
		{name: "wrong secret", secret: "s3cret", token: func(r string) string { return r }, presented: "guess", want: relay.KindUnauthorized},
		{name: "missing secret", secret: "s3cret", token: func(r string) string { return r }, want: relay.KindUnauthorized},
		{name: "right secret", secret: "s3cret", token: func(r string) string { return r }, presented: "s3cret"},
		{name: "no secret configured", token: func(r string) string { return r }, presented: "anything"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newStore()
			_, token := newSource(t, store, alice, tt.secret)
			_, err := relay.NewIngress(store, relay.DefaultLimits()).Accept(t.Context(), tt.token(token), tt.presented, relay.Record{Method: "POST"})
			if tt.want == "" {
				require.NoError(t, err)
				return
			}
			requireKind(t, tt.want, err)
		})
	}
}

func TestIngressOverCap(t *testing.T) {
	store := newStore()
	src, token := newSource(t, store, alice, "")
	limits := relay.DefaultLimits()
	limits.BytesPerAccount = 300
	ingress := relay.NewIngress(store, limits)
	rec := relay.Record{Method: "POST", Body: []byte(strings.Repeat("a", 100))}

	_, err := ingress.Accept(t.Context(), token, "", rec)
	require.NoError(t, err)
	_, err = ingress.Accept(t.Context(), token, "", rec)
	requireKind(t, relay.KindOverCap, err)

	n, _, err := store.Deliveries().Stats(t.Context(), src.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
}

func TestRedactHeaders(t *testing.T) {
	h := http.Header{
		"Authorization": {"Bearer x"},
		"Cookie":        {"a=b"},
		"X-Hive-Secret": {"s"},
		"X-Gotify-Key":  {"k"},
		"x-gotify-key":  {"k"},
		"Content-Type":  {"text/plain"},
		"X-Many":        {"one", "two"},
	}
	got := relay.RedactHeaders(h, 1<<10)
	assert.Equal(t, map[string][]string{
		"Content-Type": {"text/plain"},
		"X-Many":       {"one", "two"},
	}, got)

	// "Content-Type"+"text/plain" is 22 and "X-Many"+"one" is 9: 31 fits,
	// "two" would make 34.
	got = relay.RedactHeaders(h, 33)
	assert.Equal(t, map[string][]string{
		"Content-Type": {"text/plain"},
		"X-Many":       {"one"},
	}, got)

	assert.Empty(t, relay.RedactHeaders(h, 5))
}
