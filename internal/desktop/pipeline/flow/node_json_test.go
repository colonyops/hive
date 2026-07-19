package flow

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNode_JSONRoundTrip covers the wire format FlowsService.GetFlow/
// SaveFlow use over Wails: Node's Config is an interface field, so a plain
// reflection-based json.Marshal/Unmarshal can't round-trip it — MarshalJSON/
// UnmarshalJSON (node_json.go) do the same two-pass, flattened-fields job
// UnmarshalYAML does for the on-disk format.
func TestNode_JSONRoundTrip(t *testing.T) {
	n := Node{
		ID:   "tag",
		Type: "function",
		Name: "Tag reviewed",
		Config: &FunctionConfig{
			OnMessage: "return msg;",
			OutputsN:  2,
			Timeout:   Duration(5e9),
		},
	}

	data, err := json.Marshal(n)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"timeout":"5s"`, "Duration must serialize as a duration string, not nanoseconds")

	var decoded Node
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, n, decoded)
}

func TestNode_JSON_DisabledAndNameOmitEmpty(t *testing.T) {
	n := Node{ID: "src", Type: "github-source", Config: &GithubSourceConfig{Source: "my-source"}}
	data, err := json.Marshal(n)
	require.NoError(t, err)
	assert.NotContains(t, string(data), `"name"`)
	assert.NotContains(t, string(data), `"disabled"`)
}

func TestNode_UnmarshalJSON_UnknownType_IsHardError(t *testing.T) {
	var n Node
	err := json.Unmarshal([]byte(`{"id":"src","type":"not-a-real-type"}`), &n)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown type")
}

func TestNode_UnmarshalJSON_UnknownPerTypeField_IsHardError(t *testing.T) {
	var n Node
	err := json.Unmarshal([]byte(`{"id":"src","type":"github-source","source":"my-source","extra_field":"nope"}`), &n)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "extra_field")
}

// TestFlow_JSONRoundTrip exercises the full Flow value FlowsService moves
// over Wails, not just a single Node.
func TestFlow_JSONRoundTrip(t *testing.T) {
	f := Flow{
		ID:      "triage",
		Name:    "Frontend Triage",
		Enabled: true,
		Nodes: []Node{
			{ID: "src", Type: "github-source", Config: &GithubSourceConfig{Source: "my-source"}},
			{ID: "sink", Type: "feed", Config: &FeedConfig{Feed: "my-feed"}},
		},
		Wires: []Wire{{From: "src", To: "sink"}},
	}

	data, err := json.Marshal(f)
	require.NoError(t, err)

	var decoded Flow
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, f, decoded)
}
