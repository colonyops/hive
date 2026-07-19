package flow

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func decodeNode(t *testing.T, yamlStr string) (Node, error) {
	t.Helper()
	var n Node
	err := yaml.Unmarshal([]byte(yamlStr), &n)
	return n, err
}

func TestNode_DecodesReservedFieldsAndConfig(t *testing.T) {
	n, err := decodeNode(t, `id: src
type: github-source
name: My Source
disabled: true
source: my-source
`)
	require.NoError(t, err)
	assert.Equal(t, "src", n.ID)
	assert.Equal(t, "github-source", n.Type)
	assert.Equal(t, "My Source", n.Name)
	assert.True(t, n.Disabled)

	cfg, ok := n.Config.(*GithubSourceConfig)
	require.True(t, ok)
	assert.Equal(t, "my-source", cfg.Source)
}

func TestNode_UnknownType_IsHardError(t *testing.T) {
	_, err := decodeNode(t, `id: src
type: not-a-real-type
`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown type")
}

func TestNode_UnknownPerTypeField_IsHardError(t *testing.T) {
	_, err := decodeNode(t, `id: src
type: github-source
source: my-source
extra_field: nope
`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "extra_field")
}

func TestNode_ReservedKeysDoNotTripStrictPerTypeDecode(t *testing.T) {
	// id/type/name/disabled coexist on the same mapping as per-type fields;
	// the strict decode must not treat them as unknown fields.
	n, err := decodeNode(t, `id: tag
type: function
name: Tag reviewed
disabled: false
on_message: "return msg;"
outputs: 3
`)
	require.NoError(t, err)
	cfg, ok := n.Config.(*FunctionConfig)
	require.True(t, ok)
	assert.Equal(t, "return msg;", cfg.OnMessage)
	assert.Equal(t, 3, cfg.Outputs())
}

func TestNode_NonMapping_IsError(t *testing.T) {
	_, err := decodeNode(t, `"just a string"`)
	require.Error(t, err)
}
