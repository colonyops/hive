package actions

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func decodeAction(t *testing.T, yamlStr string) (Action, error) {
	t.Helper()
	var a Action
	err := yaml.Unmarshal([]byte(yamlStr), &a)
	return a, err
}

func TestAction_DecodesEnvelopeAndConfig(t *testing.T) {
	a, err := decodeAction(t, `id: spawn-review
label: Spawn review agent
type: launch-session
applies_to: [pr, issue]
prompt_template: "Review {{ .Payload.title }}"
agent: claude
`)
	require.NoError(t, err)
	assert.Equal(t, "spawn-review", a.ID)
	assert.Equal(t, "Spawn review agent", a.Label)
	assert.Equal(t, "launch-session", a.Type)
	assert.Equal(t, []string{"pr", "issue"}, a.AppliesTo)

	cfg, ok := a.Config.(*LaunchSessionConfig)
	require.True(t, ok)
	assert.Equal(t, "Review {{ .Payload.title }}", cfg.PromptTemplate)
	assert.Equal(t, "claude", cfg.Agent)
}

func TestAction_DecodesShellWithoutLegacyFields(t *testing.T) {
	_, err := decodeAction(t, `id: shell-echo
label: Echo
type: shell
command_template: "echo hi"
`)
	require.NoError(t, err)
}

func TestAction_UnknownType_IsHardError(t *testing.T) {
	_, err := decodeAction(t, `id: x
label: X
type: not-a-real-type
`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown type")
}

func TestAction_UnknownPerTypeField_IsHardError(t *testing.T) {
	_, err := decodeAction(t, `id: x
label: X
type: shell
command_template: "echo hi"
extra_field: nope
`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "extra_field")
}

func TestAction_ReservedKeysDoNotTripStrictPerTypeDecode(t *testing.T) {
	a, err := decodeAction(t, `id: x
label: X
type: publish-message
applies_to: [pr]
message_template: "{{ .Payload.title }}"
topic: pipeline.pr-events
`)
	require.NoError(t, err)
	cfg, ok := a.Config.(*PublishMessageConfig)
	require.True(t, ok)
	assert.Equal(t, "pipeline.pr-events", cfg.Topic)
}

func TestAction_NonMapping_IsError(t *testing.T) {
	_, err := decodeAction(t, `"just a string"`)
	require.Error(t, err)
}

func TestLaunchSessionConfig_Validate_RequiresPromptTemplate(t *testing.T) {
	cfg := &LaunchSessionConfig{}
	require.Error(t, cfg.Validate())

	cfg.PromptTemplate = "hi"
	require.NoError(t, cfg.Validate())
}

func TestShellConfig_Validate_RequiresCommandTemplate(t *testing.T) {
	cfg := &ShellConfig{}
	require.Error(t, cfg.Validate())

	cfg.CommandTemplate = "true"
	require.NoError(t, cfg.Validate())
}

func TestPublishMessageConfig_Validate_RequiresTopic(t *testing.T) {
	cfg := &PublishMessageConfig{}
	require.Error(t, cfg.Validate())

	cfg.MessageTemplate = "message"
	cfg.Topic = "some.topic"
	require.NoError(t, cfg.Validate())
}
