package config

import (
	"testing"

	"github.com/hay-kot/criterio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateTodos_EmptyActions(t *testing.T) {
	cfg := validConfig(t)
	cfg.Todos.Actions = nil

	err := cfg.validateTodos()
	assert.NoError(t, err)
}

func TestValidateTodos_ValidAction(t *testing.T) {
	cfg := validConfig(t)
	cfg.Todos.Actions = map[string]string{
		"jira": `open-jira {{ .Value | shq }}`,
	}

	err := cfg.validateTodos()
	require.NoError(t, err)
	// Keys should be normalized to lowercase
	assert.Contains(t, cfg.Todos.Actions, "jira")
}

func TestValidateTodos_NormalizesSchemeToLowercase(t *testing.T) {
	cfg := validConfig(t)
	cfg.Todos.Actions = map[string]string{
		"JIRA":   `open-jira {{ .Value | shq }}`,
		"MyTool": `my-tool {{ .URI | shq }}`,
	}

	err := cfg.validateTodos()
	require.NoError(t, err)

	assert.Contains(t, cfg.Todos.Actions, "jira")
	assert.Contains(t, cfg.Todos.Actions, "mytool")
	assert.NotContains(t, cfg.Todos.Actions, "JIRA")
	assert.NotContains(t, cfg.Todos.Actions, "MyTool")
}

func TestValidateTodos_RejectsBuiltinSchemes(t *testing.T) {
	builtins := []string{"session", "review", "code-review", "http", "https"}

	for _, scheme := range builtins {
		t.Run(scheme, func(t *testing.T) {
			cfg := validConfig(t)
			cfg.Todos.Actions = map[string]string{
				scheme: `echo {{ .Value | shq }}`,
			}

			err := cfg.validateTodos()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "cannot override built-in scheme")
		})
	}
}

func TestValidateTodos_RejectsBuiltinSchemesCaseInsensitive(t *testing.T) {
	cfg := validConfig(t)
	cfg.Todos.Actions = map[string]string{
		"SESSION": `echo {{ .Value | shq }}`,
	}

	err := cfg.validateTodos()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot override built-in scheme")
}

func TestValidateTodos_RejectsInvalidTemplate(t *testing.T) {
	cfg := validConfig(t)
	cfg.Todos.Actions = map[string]string{
		"jira": `open {{ .Value`,
	}

	err := cfg.validateTodos()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "template error")
}

func TestValidateTodos_RejectsUnquotedInterpolation(t *testing.T) {
	cfg := validConfig(t)
	cfg.Todos.Actions = map[string]string{
		"jira": `open-jira {{ .Value }}`,
	}

	err := cfg.validateTodos()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must use | shq")
}

func TestValidateTodos_AcceptsShqInterpolation(t *testing.T) {
	cfg := validConfig(t)
	cfg.Todos.Actions = map[string]string{
		"jira": `open-jira {{ .Value | shq }}`,
	}

	err := cfg.validateTodos()
	assert.NoError(t, err)
}

func TestValidateTodos_MultipleErrors(t *testing.T) {
	cfg := validConfig(t)
	cfg.Todos.Actions = map[string]string{
		"session": `echo {{ .Value | shq }}`,
		"bad":     `open {{ .Value`,
		"unsafe":  `open-jira {{ .Value }}`,
		"good":    `open-tool {{ .URI | shq }}`,
	}

	err := cfg.validateTodos()
	require.Error(t, err)

	var fieldErrs criterio.FieldErrors
	require.ErrorAs(t, err, &fieldErrs)
	assert.Len(t, fieldErrs, 3)
}

func TestValidateTodos_AllTemplateVars(t *testing.T) {
	cfg := validConfig(t)
	cfg.Todos.Actions = map[string]string{
		"custom": `tool --scheme {{ .Scheme | shq }} --value {{ .Value | shq }} --uri {{ .URI | shq }} --title {{ .Title | shq }}`,
	}

	err := cfg.validateTodos()
	assert.NoError(t, err)
}

func TestValidateTodos_RejectsDuplicateAfterNormalization(t *testing.T) {
	cfg := validConfig(t)
	cfg.Todos.Actions = map[string]string{
		"JIRA": `open-jira {{ .Value | shq }}`,
		"jira": `open-jira {{ .Value | shq }}`,
	}

	err := cfg.validateTodos()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate scheme")
}

func TestValidateDeep_IncludesTodosValidation(t *testing.T) {
	cfg := validConfig(t)
	cfg.Todos.Actions = map[string]string{
		"https": `echo {{ .Value | shq }}`,
	}

	err := cfg.ValidateDeep("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot override built-in scheme")
}
