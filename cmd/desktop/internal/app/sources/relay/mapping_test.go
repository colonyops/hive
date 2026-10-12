package relay

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/relayformat"
)

func parsedItem(t *testing.T, body string) relayformat.Item {
	t.Helper()
	items, _, err := relayformat.Parse(relayformat.DialectGeneric, relayformat.Delivery{Path: "/kuma", Body: []byte(body)})
	if err != nil {
		t.Fatal(err)
	}
	return items[0]
}

func TestApply(t *testing.T) {
	t.Parallel()

	const kuma = `{"monitor":{"id":7,"name":"api"},"heartbeat":{"status":0},"tags":["a","b"],"a/b":"slash","m~n":"tilde"}`

	tests := []struct {
		name      string
		m         *Map
		wantKey   string
		wantField string
		want      any
	}{
		{name: "pointer into raw.body sets the key and id", m: &Map{Key: "/raw/body/monitor/id"}, wantKey: "7", wantField: "id", want: "7"},
		{name: "missing path keeps the parser key", m: &Map{Key: "/raw/body/nope"}},
		{name: "object value keeps the parser key", m: &Map{Key: "/raw/body/monitor"}},
		{name: "states maps the resolved value", m: &Map{State: "/raw/body/heartbeat/status", States: map[string]string{"0": "down", "1": "resolved"}}, wantField: "state", want: "down"},
		{name: "unmapped state passes through", m: &Map{State: "/raw/body/heartbeat/status"}, wantField: "state", want: "0"},
		{name: "~1 escapes a slash", m: &Map{Title: "/raw/body/a~1b"}, wantField: "title", want: "slash"},
		{name: "~0 escapes a tilde", m: &Map{Kind: "/raw/body/m~0n"}, wantField: "kind", want: "tilde"},
		{name: "array index", m: &Map{URL: "/raw/body/tags/1"}, wantField: "url", want: "b"},
		{name: "array index out of range keeps the value", m: &Map{URL: "/raw/body/tags/9"}, wantField: "url"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			in := parsedItem(t, kuma)
			out := Apply(tc.m, in)

			wantKey := tc.wantKey
			if wantKey == "" {
				wantKey = in.Key
			}
			assert.Equal(t, wantKey, out.Key)
			if tc.wantField == "" {
				return
			}
			assert.Equal(t, tc.want, out.Payload[tc.wantField])
		})
	}
}

func TestApplyNilMapIsIdentity(t *testing.T) {
	t.Parallel()
	in := parsedItem(t, `{"id":"x"}`)
	assert.Equal(t, in, Apply(nil, in))
}

func TestApplyLeavesTheParsedPayloadAlone(t *testing.T) {
	t.Parallel()
	in := parsedItem(t, `{"id":"x","n":{"v":"y"}}`)
	before, _ := json.Marshal(in.Payload)
	Apply(&Map{Title: "/n/v"}, in)
	after, _ := json.Marshal(in.Payload)
	assert.JSONEq(t, string(before), string(after))
}
