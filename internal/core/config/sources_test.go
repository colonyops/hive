package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func boolPtr(b bool) *bool { return &b }

func TestApplyDefaults_SourceBuiltinTemplates(t *testing.T) {
	cfg := &Config{}
	cfg.applyDefaults()

	assert.NotEmpty(t, cfg.Sources.Issues.Templates.Name)
	assert.NotEmpty(t, cfg.Sources.Issues.Templates.Prompt)
	assert.NotEmpty(t, cfg.Sources.Issues.Templates.Tags)
	assert.NotEmpty(t, cfg.Sources.PRs.Templates.Name)
	assert.NotEmpty(t, cfg.Sources.PRs.Templates.Prompt)
	assert.NotEmpty(t, cfg.Sources.PRs.Templates.Tags)
}

func TestApplyDefaults_SourceBuiltinTemplatesPreserveUserValues(t *testing.T) {
	cfg := &Config{
		Sources: SourcesConfig{
			Issues: BuiltinSourceConfig{
				Templates: SourceTemplateConfig{
					Name:   "custom-{{ .Title }}",
					Prompt: "custom prompt",
					Tags:   []string{"custom"},
				},
			},
		},
	}
	cfg.applyDefaults()

	assert.Equal(t, "custom-{{ .Title }}", cfg.Sources.Issues.Templates.Name)
	assert.Equal(t, "custom prompt", cfg.Sources.Issues.Templates.Prompt)
	assert.Equal(t, []string{"custom"}, cfg.Sources.Issues.Templates.Tags)
	assert.NotEqual(t, cfg.Sources.Issues.Templates.Name, cfg.Sources.PRs.Templates.Name, "prs defaults are independent")
}

func TestConfigLoadsTopLevelSources(t *testing.T) {
	yamlSrc := `
sources:
  issues:
    enabled: true
    templates:
      name: "gh-{{ .Fields.number }}"
      prompt: "work on {{ .Title }}"
  prs:
    enabled: false
  external:
    - id: reference
      command: ["hive-reference-source"]
      templates:
        name: "ref-{{ .ID }}"
        prompt: "do {{ .Title }}"
        tags: ["ref"]
`
	var cfg Config
	require.NoError(t, yaml.Unmarshal([]byte(yamlSrc), &cfg))

	assert.True(t, *cfg.Sources.Issues.Enabled)
	assert.False(t, *cfg.Sources.PRs.Enabled)
	assert.Equal(t, "gh-{{ .Fields.number }}", cfg.Sources.Issues.Templates.Name)
	require.Len(t, cfg.Sources.External, 1)
	assert.Equal(t, "reference", cfg.Sources.External[0].ID)
	assert.Equal(t, []string{"hive-reference-source"}, cfg.Sources.External[0].Command)
	assert.Equal(t, "ref-{{ .ID }}", cfg.Sources.External[0].Templates.Name)
	assert.Equal(t, "do {{ .Title }}", cfg.Sources.External[0].Templates.Prompt)
	assert.Equal(t, []string{"ref"}, cfg.Sources.External[0].Templates.Tags)
}

func TestConfigLoadsTopLevelSources_WrongNestingIgnored(t *testing.T) {
	yamlSrc := `
plugins:
  sources:
    issues:
      enabled: true
`
	var cfg Config
	require.NoError(t, yaml.Unmarshal([]byte(yamlSrc), &cfg))

	assert.Empty(t, cfg.Sources.External)
	assert.Nil(t, cfg.Sources.Issues.Enabled)
	assert.Nil(t, cfg.Sources.PRs.Enabled)
}

func TestValidateSources_ValidBuiltins(t *testing.T) {
	cfg := validConfig(t)
	cfg.Sources = SourcesConfig{
		Issues: BuiltinSourceConfig{
			Enabled: boolPtr(true),
			Templates: SourceTemplateConfig{
				Name:   "gh-{{ .Fields.number }}-{{ .Title }}",
				Prompt: "Work on {{ .Title }}\n\n{{ .Fields.url }}",
				Tags:   []string{"github", "issue-{{ .Fields.number }}"},
			},
		},
		PRs: BuiltinSourceConfig{
			Enabled: boolPtr(true),
			Templates: SourceTemplateConfig{
				Name:   "gh-pr-{{ .Fields.number }}",
				Prompt: "Review {{ .Title }}",
				Tags:   []string{"pr"},
			},
		},
	}

	require.NoError(t, cfg.Validate())
}

func TestValidateSources_ExternalValidation(t *testing.T) {
	tests := []struct {
		name     string
		external []ExternalSourceConfig
		wantErr  string
	}{
		{name: "missing id", external: []ExternalSourceConfig{{Command: []string{"cmd"}}}, wantErr: "id"},
		{name: "invalid id", external: []ExternalSourceConfig{{ID: "Not_Valid!", Command: []string{"cmd"}}}, wantErr: "invalid id"},
		{name: "duplicate id", external: []ExternalSourceConfig{{ID: "dup", Command: []string{"cmd"}}, {ID: "dup", Command: []string{"cmd2"}}}, wantErr: "duplicate"},
		{name: "builtin collision", external: []ExternalSourceConfig{{ID: "issues", Command: []string{"cmd"}}}, wantErr: "collides with the built-in"},
		{name: "missing command", external: []ExternalSourceConfig{{ID: "reference"}}, wantErr: "command"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig(t)
			cfg.Sources.External = tt.external

			err := cfg.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestValidateSources_ExternalBuiltinIDAllowedWhenBuiltinDisabled(t *testing.T) {
	cfg := validConfig(t)
	cfg.Sources.Issues.Enabled = boolPtr(false)
	cfg.Sources.External = []ExternalSourceConfig{{ID: "issues", Command: []string{"my-issues"}}}

	require.NoError(t, cfg.Validate())
}

func TestValidateSources_DisabledExternalSkipsCommandRequirement(t *testing.T) {
	cfg := validConfig(t)
	cfg.Sources.External = []ExternalSourceConfig{{ID: "reference", Enabled: boolPtr(false)}}

	require.NoError(t, cfg.Validate())
}

func TestValidateSources_InvalidTemplateSyntax(t *testing.T) {
	cfg := validConfig(t)
	cfg.Sources.External = []ExternalSourceConfig{{
		ID:      "reference",
		Command: []string{"cmd"},
		Templates: SourceTemplateConfig{
			Name: "{{ .Title",
		},
	}}

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "templates")
}

func TestValidateSources_MissingFieldsKeyIsNotAConfigError(t *testing.T) {
	// .Fields.<key> is a dynamic, per-item map; referencing an unknown key is
	// only a render-time failure (RenderSessionTemplates), never a config
	// validation error.
	cfg := validConfig(t)
	cfg.Sources.Issues.Templates = SourceTemplateConfig{
		Name:   "{{ .Fields.whatever }}",
		Prompt: "{{ .Fields.anything }}",
	}

	require.NoError(t, cfg.Validate())
}
