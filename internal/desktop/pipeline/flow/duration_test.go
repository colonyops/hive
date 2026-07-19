package flow

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func decodeDuration(t *testing.T, s string) (Duration, error) {
	t.Helper()
	var node yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte(s), &node))
	var d Duration
	err := node.Content[0].Decode(&d)
	return d, err
}

func TestDuration_DecodesDurationString(t *testing.T) {
	d, err := decodeDuration(t, `"5s"`)
	require.NoError(t, err)
	assert.Equal(t, 5*time.Second, d.Duration())
}

func TestDuration_RejectsBareInteger(t *testing.T) {
	_, err := decodeDuration(t, `5`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bare number")
}

func TestDuration_RejectsBareFloat(t *testing.T) {
	_, err := decodeDuration(t, `5.5`)
	require.Error(t, err)
}

func TestDuration_RejectsUnparseableString(t *testing.T) {
	_, err := decodeDuration(t, `"not-a-duration"`)
	require.Error(t, err)
}
