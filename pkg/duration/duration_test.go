package duration

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/invopop/jsonschema"
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

func TestDuration_MarshalYAML_ProducesDurationString(t *testing.T) {
	out, err := yaml.Marshal(Duration(5 * time.Second))
	require.NoError(t, err)
	assert.Equal(t, "5s\n", string(out))
}

func TestDuration_MarshalText_UnmarshalText_RoundTrip(t *testing.T) {
	d := Duration(250 * time.Millisecond)
	text, err := d.MarshalText()
	require.NoError(t, err)
	assert.Equal(t, "250ms", string(text))

	var decoded Duration
	require.NoError(t, decoded.UnmarshalText(text))
	assert.Equal(t, d, decoded)
}

func TestDuration_RejectsBareZero(t *testing.T) {
	_, err := decodeDuration(t, `0`)
	require.Error(t, err)
}

func TestDuration_JSON_RoundTrip(t *testing.T) {
	type wire struct {
		Timeout Duration `json:"timeout"`
	}
	data, err := json.Marshal(wire{Timeout: Duration(90 * time.Second)})
	require.NoError(t, err)
	assert.JSONEq(t, `{"timeout":"1m30s"}`, string(data))

	var decoded wire
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, 90*time.Second, decoded.Timeout.Duration())

	require.Error(t, json.Unmarshal([]byte(`{"timeout":5}`), &decoded))
}

func TestDuration_JSONSchema_ReflectsAsString(t *testing.T) {
	type config struct {
		Interval Duration `json:"interval"`
	}
	schema := (&jsonschema.Reflector{DoNotReference: true}).Reflect(&config{})
	prop, ok := schema.Properties.Get("interval")
	require.True(t, ok)
	assert.Equal(t, "string", prop.Type)
	assert.Regexp(t, prop.Pattern, "1h30m")
	assert.NotRegexp(t, prop.Pattern, "5")
}
