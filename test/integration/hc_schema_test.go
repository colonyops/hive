//go:build integration

package integration

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The CLI prints a command's error on stdout (cmd/hive/cli/cli.go), so the
// assertions below read the returned output, not stderr.

func TestHCCreateDryRunValid(t *testing.T) {
	h := NewHarness(t)

	input := `{"title":"Auth","type":"epic","children":[
		{"ref":"jwt","title":"JWT middleware","type":"task"},
		{"title":"Login endpoint","type":"task","blockers":["jwt"]}]}`

	out, err := h.RunWithStdin(input, "hc", "create", "--dry-run")
	require.NoError(t, err, out)

	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &result))
	assert.Equal(t, true, result["valid"])
	assert.Equal(t, float64(3), result["items"])

	items, err := h.RunJSONLines("hc", "list", "--all", "--json")
	require.NoError(t, err)
	assert.Empty(t, items, "a dry run must create nothing")
}

func TestHCCreateRejectsUnknownKey(t *testing.T) {
	h := NewHarness(t)

	input := `{"title":"Auth","type":"epic","children":[{"title":"x","type":"task","descr":"typo"}]}`

	out, err := h.RunWithStdin(input, "hc", "create")
	require.Error(t, err)
	assert.Contains(t, out, "children[0].descr")
	assert.Contains(t, out, "unknown key")
}

func TestHCCreateRejectsBadEnum(t *testing.T) {
	h := NewHarness(t)

	input := `{"title":"Auth","type":"epic","children":[{"title":"x","type":"tsak"}]}`

	out, err := h.RunWithStdin(input, "hc", "create", "--dry-run")
	require.Error(t, err)
	assert.Contains(t, out, "children[0].type")
}

func TestHCCreateRejectsDuplicateRef(t *testing.T) {
	h := NewHarness(t)

	input := `{"title":"Auth","type":"epic","children":[
		{"ref":"a","title":"x","type":"task"},
		{"ref":"a","title":"y","type":"task"}]}`

	out, err := h.RunWithStdin(input, "hc", "create")
	require.Error(t, err)
	assert.Contains(t, out, "children[1].ref")
	assert.Contains(t, out, "duplicate ref")

	items, err := h.RunJSONLines("hc", "list", "--all", "--json")
	require.NoError(t, err)
	assert.Empty(t, items, "a rejected tree must create nothing")
}
