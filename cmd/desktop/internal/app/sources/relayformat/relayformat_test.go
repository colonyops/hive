package relayformat

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func delivery(body string) Delivery {
	return Delivery{
		ID:      "0192",
		Method:  "POST",
		Path:    "/alerts",
		Query:   url.Values{"q": {"1"}},
		Headers: map[string][]string{"Content-Type": {"application/json"}},
		Body:    []byte(body),
	}
}

func parseOne(t *testing.T, dialect string, d Delivery) Item {
	t.Helper()
	items, matched, err := Parse(dialect, d)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, DialectGeneric, matched)
	return items[0]
}

func sha(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}

func TestParseGeneric(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		body      string
		wantKey   string
		wantTitle string
		wantURL   any
		wantBody  any
	}{
		{
			name:      "string id",
			body:      `{"id":" abc ","title":"Deploy failed","url":"https://ci.example.com/1","state":"open","repo":"api"}`,
			wantKey:   "abc",
			wantTitle: "Deploy failed",
			wantURL:   "https://ci.example.com/1",
		},
		{
			name:      "numeric id",
			body:      `{"id":12345678901234567890}`,
			wantKey:   "12345678901234567890",
			wantTitle: "/alerts · 123456789012",
		},
		{
			name:      "no id hashes the body",
			body:      `{"title":"t"}`,
			wantKey:   sha(`{"title":"t"}`),
			wantTitle: "t",
		},
		{
			name:      "text body",
			body:      "disk full",
			wantKey:   sha("disk full"),
			wantTitle: "/alerts · " + sha("disk full")[:12],
			wantBody:  "disk full",
		},
		{
			name:      "array body is treated as text",
			body:      `[1,2]`,
			wantKey:   sha(`[1,2]`),
			wantTitle: "/alerts · " + sha(`[1,2]`)[:12],
			wantBody:  `[1,2]`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			item := parseOne(t, DialectGeneric, delivery(tc.body))
			assert.Equal(t, tc.wantKey, item.Key)
			assert.Equal(t, tc.wantTitle, item.Payload["title"])
			assert.Equal(t, tc.wantURL, item.Payload["url"])
			assert.Equal(t, tc.wantBody, item.Payload["body"])
			assert.Equal(t, DefaultKind, item.Payload["kind"])
			assert.NotNil(t, item.Payload["id"])
		})
	}
}

func TestParseKeepsPayloadFields(t *testing.T) {
	t.Parallel()
	item := parseOne(t, DialectAuto, delivery(`{"id":"x","kind":"Alert","state":"resolved","repo":"api","body":"b"}`))
	assert.Equal(t, "Alert", item.Payload["kind"])
	assert.Equal(t, "resolved", item.Payload["state"])
	assert.Equal(t, "api", item.Payload["repo"])
	assert.Equal(t, "b", item.Payload["body"])
}

func TestParseExactRetryCollapsesToOneKey(t *testing.T) {
	t.Parallel()
	first := parseOne(t, DialectGeneric, delivery(`{"msg":"same"}`))
	retry := delivery(`{"msg":"same"}`)
	retry.ID = "0193"
	second := parseOne(t, DialectGeneric, retry)
	assert.Equal(t, first.Key, second.Key)
}

func TestParseRawBody(t *testing.T) {
	t.Parallel()

	jsonItem := parseOne(t, DialectGeneric, delivery(`{"monitor":{"id":7}}`))
	raw, ok := jsonItem.Payload["raw"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, map[string]any{"monitor": map[string]any{"id": json.Number("7")}}, raw["body"])
	assert.Equal(t, "POST", raw["method"])
	assert.Equal(t, "/alerts", raw["path"])
	assert.Equal(t, map[string]any{"q": []any{"1"}}, raw["query"])
	assert.Equal(t, map[string]any{"Content-Type": []any{"application/json"}}, raw["headers"])

	textItem := parseOne(t, DialectGeneric, delivery("plain"))
	textRaw, ok := textItem.Payload["raw"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "plain", textRaw["body"])
}

func TestParseRejectsUnknownDialect(t *testing.T) {
	t.Parallel()
	_, _, err := Parse("bogus", delivery(`{}`))
	require.ErrorContains(t, err, `unknown dialect "bogus"`)
}

func TestGenericIdentityEmptyPathTitle(t *testing.T) {
	t.Parallel()
	d := delivery(`{"id":"k"}`)
	d.Path = ""
	item := parseOne(t, DialectGeneric, d)
	assert.Equal(t, "/ · k", item.Payload["title"])
}
