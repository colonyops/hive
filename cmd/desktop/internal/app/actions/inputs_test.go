package actions

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAction_DecodesEnvelopeInputs(t *testing.T) {
	a, err := decodeAction(t, `id: ignore-alert
label: Ignore alert
type: clipboard
inputs:
  - name: reason
    label: Reason
    required: true
    placeholder: why this is being silenced
  - name: duration
    type: select
    default: 1h
    options: [1h, 24h]
text_template: "flux suspend # {{ .Inputs.reason }}"
`)
	require.NoError(t, err)
	require.Len(t, a.Inputs, 2)

	assert.Equal(t, "reason", a.Inputs[0].Name)
	assert.Equal(t, "Reason", a.Inputs[0].Label)
	assert.True(t, a.Inputs[0].Required)
	assert.Equal(t, InputTypeText, a.Inputs[0].Type, "an omitted type defaults to text at the decode boundary")
	assert.Equal(t, InputTypeSelect, a.Inputs[1].Type)
	assert.Equal(t, []string{"1h", "24h"}, a.Inputs[1].Options)
}

func TestValidateInputs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		inputs  []InputSpec
		wantErr string
	}{
		{
			name:   "valid",
			inputs: []InputSpec{{Name: "reason", Type: InputTypeText}, {Name: "level", Type: InputTypeSelect, Options: []string{"a", "b"}, Default: "a"}},
		},
		{
			name:    "name must be a template identifier",
			inputs:  []InputSpec{{Name: "the-reason", Type: InputTypeText}},
			wantErr: "template identifier",
		},
		{
			name:    "duplicate names",
			inputs:  []InputSpec{{Name: "reason", Type: InputTypeText}, {Name: "reason", Type: InputTypeText}},
			wantErr: "duplicate input name",
		},
		{
			name:    "unknown type",
			inputs:  []InputSpec{{Name: "reason", Type: "date"}},
			wantErr: `unknown type "date"`,
		},
		{
			name:    "select without options",
			inputs:  []InputSpec{{Name: "level", Type: InputTypeSelect}},
			wantErr: "requires options",
		},
		{
			name:    "options on a text input",
			inputs:  []InputSpec{{Name: "reason", Type: InputTypeText, Options: []string{"a"}}},
			wantErr: "only valid on a select",
		},
		{
			name:    "default outside the options",
			inputs:  []InputSpec{{Name: "level", Type: InputTypeSelect, Options: []string{"a"}, Default: "z"}},
			wantErr: "not one of its options",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateInputs(tc.inputs)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestResolveInputs(t *testing.T) {
	t.Parallel()

	action := Action{
		ID: "ignore",
		Inputs: []InputSpec{
			{Name: "reason", Type: InputTypeText, Required: true},
			{Name: "window", Type: InputTypeSelect, Options: []string{"1h", "24h"}, Default: "1h"},
			{Name: "note", Type: InputTypeMultiline},
		},
	}

	t.Run("fills blanks from defaults", func(t *testing.T) {
		t.Parallel()
		resolved, err := action.ResolveInputs(map[string]string{"reason": "flapping"})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"reason": "flapping", "window": "1h", "note": ""}, resolved)
	})

	t.Run("a required input with no value is refused", func(t *testing.T) {
		t.Parallel()
		_, err := action.ResolveInputs(map[string]string{"reason": "   "})
		require.ErrorContains(t, err, `input "reason" is required`)
	})

	t.Run("a select value outside its options is refused", func(t *testing.T) {
		t.Parallel()
		_, err := action.ResolveInputs(map[string]string{"reason": "flapping", "window": "7d"})
		require.ErrorContains(t, err, "not one of its options")
	})

	t.Run("an undeclared name is refused rather than ignored", func(t *testing.T) {
		t.Parallel()
		_, err := action.ResolveInputs(map[string]string{"reason": "flapping", "sudo": "true"})
		require.ErrorContains(t, err, `unknown input "sudo"`)
	})

	t.Run("an action declaring none resolves to nil", func(t *testing.T) {
		t.Parallel()
		resolved, err := Action{ID: "plain"}.ResolveInputs(nil)
		require.NoError(t, err)
		assert.Nil(t, resolved)
	})
}

// A flow node fires an action with nobody to ask, so a required input with no
// default is what makes an action detail-pane only — the same gate
// repo_template-less launch-session actions already sit behind.
func TestHeadlessCapable_RequiredInputWithoutDefault(t *testing.T) {
	t.Parallel()

	action := Action{ID: "notify", Type: "publish-message", Config: &PublishMessageConfig{MessageTemplate: "x", Topic: "t"}}
	assert.True(t, action.HeadlessCapable())

	action.Inputs = []InputSpec{{Name: "reason", Type: InputTypeText, Required: true}}
	assert.False(t, action.HeadlessCapable())

	action.Inputs[0].Default = "unspecified"
	assert.True(t, action.HeadlessCapable(), "a default is a value a flow worker can supply")

	action.Inputs[0].Required = false
	action.Inputs[0].Default = ""
	assert.True(t, action.HeadlessCapable())
}

func TestRunsWithoutInput(t *testing.T) {
	t.Parallel()

	clipboard := Action{ID: "copy", Type: "clipboard", Config: &ClipboardConfig{TextTemplate: "x"}}
	assert.True(t, clipboard.RunsWithoutInput(), "a clipboard render needs no flow worker")

	clipboard.Inputs = []InputSpec{{Name: "branch", Type: InputTypeText, Required: true}}
	assert.False(t, clipboard.RunsWithoutInput())

	clipboard.Inputs[0].Default = "main"
	assert.True(t, clipboard.RunsWithoutInput())

	interactive := Action{ID: "launch", Type: "launch-session", Config: &LaunchSessionConfig{}}
	assert.False(t, interactive.RunsWithoutInput(), "the New Session dialog is input")
}

func TestActionEnvelopeInputsRoundTripThroughTheYAMLWriterAndLoader(t *testing.T) {
	t.Parallel()

	original := Action{
		ID:           "ignore-alert",
		Label:        "Ignore alert",
		Type:         "clipboard",
		ShowInDetail: true,
		Inputs: []InputSpec{
			{Name: "reason", Label: "Reason", Type: InputTypeMultiline, Required: true, Placeholder: "why"},
			{Name: "window", Type: InputTypeSelect, Default: "1h", Options: []string{"1h", "24h"}},
		},
		Config: &ClipboardConfig{TextTemplate: "flux suspend # {{ .Inputs.reason }}"},
	}

	s := NewActionStore(filepath.Join(t.TempDir(), "actions.yml"))
	s.mu.Lock()
	_, err := s.mutateLocked("create", original.ID, original)
	s.mu.Unlock()
	require.NoError(t, err)

	reloaded, ok := s.Get(original.ID)
	require.True(t, ok)
	assert.Equal(t, original.Inputs, reloaded.Inputs)

	editable, err := editableFromAction(reloaded)
	require.NoError(t, err)
	assert.Equal(t, original.Inputs, editable.Inputs)

	back, err := actionFromEditable(editable)
	require.NoError(t, err)
	assert.Equal(t, original.Inputs, back.Inputs)
}

func TestActionFromEditable_RejectsInvalidInputs(t *testing.T) {
	t.Parallel()

	_, err := actionFromEditable(EditableAction{
		ID:        "ignore-alert",
		Label:     "Ignore alert",
		Type:      "clipboard",
		Inputs:    []InputSpec{{Name: "not an identifier"}},
		Clipboard: &EditableClipboardConfig{TextTemplate: "x"},
	})
	require.ErrorContains(t, err, "template identifier")
}
