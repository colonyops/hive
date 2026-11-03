package ops_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/ops"
)

type sampleIn struct {
	Caller string  `json:"caller"`
	Mode   string  `json:"mode,omitempty"   jsonschema:"How."`
	Share  float64 `json:"share"`
	Paused *bool   `json:"paused,omitempty"`
}

var sample = ops.Spec[sampleIn]{
	Name: "canvas_block.put", Summary: "Put",
	Fields: ops.Constraints{
		"mode":  {Enum: []string{"a", "b"}, Default: "a"},
		"share": ops.Range(0.2, 0.8),
	},
}

func TestInfoSplitsTheLastDot(t *testing.T) {
	info := sample.Info()
	assert.Equal(t, "canvas_block", info.Resource)
	assert.Equal(t, "put", info.Verb)
	assert.Equal(t, "canvas_block_put", info.ToolName)
}

func TestInfoRefusesAVerbOutsideTheVocabulary(t *testing.T) {
	assert.Panics(t, func() { ops.Spec[sampleIn]{Name: "tab.show"}.Info() })
	assert.Panics(t, func() { ops.Spec[sampleIn]{Name: "tab"}.Info() })
}

func TestInputSchemaAppliesConstraints(t *testing.T) {
	schema, err := sample.InputSchema()
	require.NoError(t, err)

	mode := schema.Properties["mode"]
	assert.Equal(t, []any{"a", "b"}, mode.Enum)
	assert.JSONEq(t, `"a"`, string(mode.Default))
	assert.InDelta(t, 0.2, *schema.Properties["share"].Minimum, 0)
	assert.InDelta(t, 0.8, *schema.Properties["share"].Maximum, 0)

	raw, err := json.Marshal(schema)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"required":["caller","share"]`)
}

func TestAConstraintOnAMissingFieldFails(t *testing.T) {
	bad := ops.Spec[sampleIn]{Name: "tab.open", Fields: ops.Constraints{"placment": {Enum: []string{"x"}}}}
	_, err := bad.InputSchema()
	require.ErrorContains(t, err, `"placment"`)
}

func TestFields(t *testing.T) {
	fields, err := ops.Fields(sample)
	require.NoError(t, err)
	require.Len(t, fields, 4)

	assert.True(t, fields[0].Required)
	assert.False(t, fields[1].Required)
	assert.Equal(t, "How.", fields[1].Description)
	assert.True(t, fields[3].Pointer)
	assert.False(t, fields[3].Required)

	require.NoError(t, fields[1].Check("b"))
	require.EqualError(t, fields[1].Check("c"), `invalid: mode "c" is not one of a, b`)
	require.EqualError(t, fields[2].Check(0.95), "invalid: share 0.95 is outside 0.2 to 0.8")
}

func TestExitCodes(t *testing.T) {
	for msg, want := range map[string]int{
		"invalid: x":         2,
		"not_found: x":       3,
		"conflict: x":        4,
		"unavailable: x":     5,
		"unauthenticated: x": 6,
		"internal: x":        1,
	} {
		kind, ok := ops.ParseKind(msg)
		require.True(t, ok, msg)
		assert.Equal(t, want, ops.ExitCode(kind), msg)
	}
	_, ok := ops.ParseKind("Hive Desktop is not running")
	assert.False(t, ok)
	assert.Equal(t, 1, ops.ExitCode("other"))
}
