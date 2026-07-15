//go:build integration

package integration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExternalSourcesConfigLoadsInDeclarationOrder(t *testing.T) {
	h := NewHarness(t).WithConfig(`
version: "0.2.7"
git_path: git
agents:
  default: testbash
  testbash:
    command: bash
sources:
  external:
    - name: alerts
      command: [alert-source, --format, json]
      env:
        ALERT_CONTEXT: production
      timeout: 30s
      templates:
        name: "alert-{{ .ID }}"
        prompt: "Investigate {{ .Title }}"
        tags: [alert, "{{ .Fields.signal }}"]
    - name: incidents
      command: [incident-source]
`)

	resolved, err := h.RunJSON("config")
	require.NoError(t, err)

	sourceConfig, ok := resolved["sources"].(map[string]any)
	require.True(t, ok)
	external, ok := sourceConfig["external"].([]any)
	require.True(t, ok)
	require.Len(t, external, 2)

	alerts, ok := external[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "alerts", alerts["name"])
	assert.Equal(t, []any{"alert-source", "--format", "json"}, alerts["command"])
	assert.Equal(t, map[string]any{"ALERT_CONTEXT": "production"}, alerts["env"])
	assert.Equal(t, float64(30*time.Second), alerts["timeout"])

	templates, ok := alerts["templates"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "alert-{{ .ID }}", templates["name"])
	assert.Equal(t, "Investigate {{ .Title }}", templates["prompt"])
	assert.Equal(t, []any{"alert", "{{ .Fields.signal }}"}, templates["tags"])

	incidents, ok := external[1].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "incidents", incidents["name"])
	assert.Equal(t, []any{"incident-source"}, incidents["command"])
	assert.NotContains(t, incidents, "timeout")
}
