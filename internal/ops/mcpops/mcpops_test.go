package mcpops_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/ops"
	"github.com/colonyops/hive/internal/ops/mcpops"
)

type resizeIn struct {
	Tab   string  `json:"tab"`
	Share float64 `json:"share"`
}

func (in resizeIn) Validate() error {
	if in.Tab == "nope" {
		return errors.New("tab nope is reserved")
	}
	return nil
}

type resizeOut struct {
	Tab string `json:"tab"`
}

var resize = ops.Spec[resizeIn]{
	Name: "split.resize", Effect: ops.EffectWrite, Summary: "Resize a split", Guidance: "Sizes it.",
	Fields: ops.Constraints{"share": ops.Range(0.2, 0.8)},
}

func connect(t *testing.T, mount func(*mcp.Server)) *mcp.ClientSession {
	t.Helper()
	srv := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "test"}, nil)
	mount(srv)
	ct, st := mcp.NewInMemoryTransports()
	_, err := srv.Connect(t.Context(), st, nil)
	require.NoError(t, err)
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "test"}, nil).Connect(t.Context(), ct, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func text(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

func TestMount(t *testing.T) {
	mapped := errors.New("handler failed")
	cs := connect(t, func(srv *mcp.Server) {
		mcpops.Mount(srv, resize, func(_ context.Context, in resizeIn) (resizeOut, error) {
			switch in.Tab {
			case "gone":
				return resizeOut{}, ops.Errorf(ops.KindNotFound, "no tab %s", in.Tab)
			case "boom":
				return resizeOut{}, mapped
			}
			return resizeOut{Tab: in.Tab}, nil
		}, func(err error) error { return errors.New("internal: mapped " + err.Error()) })
	})

	tools, err := cs.ListTools(t.Context(), nil)
	require.NoError(t, err)
	require.Len(t, tools.Tools, 1)
	tool := tools.Tools[0]
	assert.Equal(t, "split_resize", tool.Name)
	assert.Equal(t, "Resize a split", tool.Title)
	assert.Equal(t, "Resize a split. Sizes it.", tool.Description)
	assert.False(t, tool.Annotations.ReadOnlyHint)
	assert.NotNil(t, tool.OutputSchema)

	tests := []struct {
		name string
		args map[string]any
		want string
	}{
		{"constraint", map[string]any{"tab": "a", "share": 0.95}, "invalid: share 0.95 is outside 0.2 to 0.8"},
		{"schema", map[string]any{"share": 0.5}, "invalid: "},
		{"validator", map[string]any{"tab": "nope", "share": 0.5}, "invalid: tab nope is reserved"},
		{"ops error", map[string]any{"tab": "gone", "share": 0.5}, "not_found: no tab gone"},
		{"mapped error", map[string]any{"tab": "boom", "share": 0.5}, "internal: mapped handler failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "split_resize", Arguments: tt.args})
			require.NoError(t, err)
			require.True(t, res.IsError)
			assert.True(t, strings.HasPrefix(text(res), tt.want), text(res))
		})
	}

	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "split_resize", Arguments: map[string]any{"tab": "a", "share": 0.5}})
	require.NoError(t, err)
	require.False(t, res.IsError, text(res))
	assert.JSONEq(t, `{"tab":"a"}`, text(res))
	assert.Equal(t, map[string]any{"tab": "a"}, res.StructuredContent)
}

func TestMountResultKeepsTheHandlersContent(t *testing.T) {
	cs := connect(t, func(srv *mcp.Server) {
		mcpops.MountResult(srv, ops.Spec[resizeIn]{Name: "image.get", Summary: "Get an image"},
			func(context.Context, resizeIn) (*mcp.CallToolResult, any, error) {
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.ImageContent{Data: []byte{1}, MIMEType: "image/png"}}}, nil, nil
			}, func(err error) error { return err })
	})

	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "image_get", Arguments: map[string]any{"tab": "a", "share": 0}})
	require.NoError(t, err)
	require.False(t, res.IsError, text(res))
	require.Len(t, res.Content, 1)
	assert.IsType(t, &mcp.ImageContent{}, res.Content[0])
	assert.Nil(t, res.StructuredContent)
}
