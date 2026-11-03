package opscli_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/cmd/hive/internal/opscli"
	"github.com/colonyops/hive/internal/ops"
	"github.com/colonyops/hive/internal/ops/sessions"
)

type block struct {
	ID string `json:"id"`
}

type putIn struct {
	Caller string         `json:"caller"`
	Canvas string         `json:"canvas"`
	ItemID int64          `json:"itemId,omitempty"`
	Tags   []string       `json:"tags,omitempty"`
	Blocks []block        `json:"blocks"`
	Front  map[string]any `json:"front,omitempty"`
}

var put = ops.Spec[putIn]{
	Name: "canvas_blocks.put", Owner: ops.OwnerDesktop, Effect: ops.EffectWrite,
	Toolset: ops.Toolset{Name: "drive", MCPPath: "/mcp/drive"},
	Summary: "Put blocks", Guidance: "Writes blocks.", Args: []string{"canvas"},
}

func run(t *testing.T, stdin string, args ...string) (map[string]any, string, error) {
	t.Helper()
	var got map[string]any
	groups, err := opscli.DesktopCommands([]ops.Op{put},
		func(_ context.Context, _ *cli.Command, _, _ string, in map[string]any) (any, error) {
			got = in
			return map[string]any{"ok": true}, nil
		},
		func(*cli.Command) (string, error) { return "me", nil })
	require.NoError(t, err)
	var out bytes.Buffer
	root := &cli.Command{Name: "hive", Writer: &out, Reader: strings.NewReader(stdin), Commands: groups}
	err = root.Run(context.Background(), append([]string{"hive"}, args...))
	return got, out.String(), err
}

func TestKebab(t *testing.T) {
	assert.Equal(t, "profile-id", opscli.Kebab("profileId"))
	assert.Equal(t, "canvas-block", opscli.Kebab("canvas_block"))
	assert.Equal(t, "image-base64", opscli.Kebab("imageBase64"))
}

func TestInputFileForNestedFields(t *testing.T) {
	file := filepath.Join(t.TempDir(), "in.json")
	require.NoError(t, os.WriteFile(file, []byte(`{"blocks":[{"id":"a"}],"front":{"x":1}}`), 0o600))

	got, out, err := run(t, "", "canvas-blocks", "put", "plan", "--input", "@"+file, "--item-id", "7", "--tags", "a", "--tags", "b")
	require.NoError(t, err)
	assert.JSONEq(t, `{"ok":true}`, out)
	assert.Equal(t, map[string]any{
		"caller": "me", "canvas": "plan", "itemId": int64(7), "tags": []string{"a", "b"},
		"blocks": []any{map[string]any{"id": "a"}}, "front": map[string]any{"x": float64(1)},
	}, got)
}

func TestInputFromStdin(t *testing.T) {
	got, _, err := run(t, `{"canvas":"plan","blocks":[]}`, "canvas-blocks", "put", "--input", "-")
	require.NoError(t, err)
	assert.Equal(t, "plan", got["canvas"])
}

func TestMissingRequiredFieldIsInvalid(t *testing.T) {
	_, _, err := run(t, "", "canvas-blocks", "put", "plan")
	require.EqualError(t, err, "invalid: blocks (set it with --input) is required")
}

func TestNestedFieldsHaveNoFlagAndSayHowToSetThem(t *testing.T) {
	var help bytes.Buffer
	groups, err := opscli.DesktopCommands([]ops.Op{put}, nil, nil)
	require.NoError(t, err)
	root := &cli.Command{Name: "hive", Writer: &help, Commands: groups}
	require.NoError(t, root.Run(context.Background(), []string{"hive", "canvas-blocks", "put", "--help"}))
	assert.Contains(t, help.String(), "Set blocks, front with --input.")
	assert.NotContains(t, help.String(), "--blocks")
	assert.Contains(t, help.String(), "--tags")
	assert.NotContains(t, help.String(), "--caller", "the caller field has no flag of its own")
}

func TestAFieldFlagMustNotHideAParentFlag(t *testing.T) {
	_, err := opscli.DesktopCommands([]ops.Op{put}, nil, nil, "tags")
	require.ErrorContains(t, err, "flag --tags is reserved")
}

func TestInputNullIsInvalid(t *testing.T) {
	_, _, err := run(t, "", "canvas-blocks", "put", "--input", "null")
	require.EqualError(t, err, "invalid: --input is not a JSON object")
}

func TestCrossFieldRulesRunBeforeTheCall(t *testing.T) {
	called := false
	groups, err := opscli.DesktopCommands([]ops.Op{sessions.SessionTagsUpdate},
		func(context.Context, *cli.Command, string, string, map[string]any) (any, error) {
			called = true
			return nil, nil
		}, nil)
	require.NoError(t, err)
	root := &cli.Command{Name: "hive", Writer: &bytes.Buffer{}, Commands: groups}
	err = root.Run(context.Background(), []string{"hive", "session-tags", "update", "s1"})
	require.EqualError(t, err, "invalid: provide add or remove")
	assert.False(t, called)
}
