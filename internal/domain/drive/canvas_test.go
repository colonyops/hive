package drive

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanvasJSONRoundTrip(t *testing.T) {
	in := Canvas{
		Space: "colonyops/hive", Path: "canvases/perf.canvas.json", Title: "Perf",
		Session: "chat-1", HiveSession: "abc123", CreatedAt: 1, UpdatedAt: 2,
		Frontmatter: Frontmatter{"tags": []any{"release"}, "ready": true},
		Blocks: []Block{
			{ID: "a", Kind: BlockKindMarkdown, Body: "# hi", CreatedAt: 1, UpdatedAt: 2},
			{ID: "b", Kind: BlockKindLink, Title: "Docs", URL: "https://example.com", CreatedAt: 1, UpdatedAt: 1},
			{ID: "c", Kind: BlockKindHTML, Body: "<b>x</b>", CreatedAt: 2, UpdatedAt: 2},
		},
	}
	data, err := json.Marshal(in)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"space":"colonyops/hive"`)
	assert.Contains(t, string(data), `"path":"canvases/perf.canvas.json"`)
	assert.NotContains(t, string(data), `"workspace"`)

	var out Canvas
	require.NoError(t, json.Unmarshal(data, &out))
	assert.Equal(t, in, out)
}

func TestCanvasUnmarshalAcceptsLegacyNumericSession(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"number", `{"session": 42, "blocks": []}`, "42"},
		{"string", `{"session": "chat-1", "blocks": []}`, "chat-1"},
		{"null", `{"session": null, "blocks": []}`, ""},
		{"absent", `{"blocks": []}`, ""},
		{"zero", `{"session": 0, "blocks": []}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c Canvas
			require.NoError(t, json.Unmarshal([]byte(tt.raw), &c))
			assert.Equal(t, tt.want, c.Session)
			assert.NotNil(t, c.Blocks)
		})
	}

	var c Canvas
	require.Error(t, json.Unmarshal([]byte(`{"session": true}`), &c))
}

func TestCanvasOmitsEmptyFrontmatter(t *testing.T) {
	data, err := json.Marshal(Canvas{Blocks: []Block{}})
	require.NoError(t, err)
	assert.JSONEq(t, `{"space":"","path":"","session":"","createdAt":0,"updatedAt":0,"blocks":[]}`, string(data))
}
