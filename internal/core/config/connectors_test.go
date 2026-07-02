package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func boolPtr(b bool) *bool { return &b }

func TestApplyDefaults_ConnectorGitHubTemplates(t *testing.T) {
	cfg := &Config{}
	cfg.applyDefaults()

	assert.NotEmpty(t, cfg.Connectors.GitHub.Templates.Name)
	assert.NotEmpty(t, cfg.Connectors.GitHub.Templates.Prompt)
	assert.NotEmpty(t, cfg.Connectors.GitHub.Templates.Tags)
}

func TestApplyDefaults_ConnectorGitHubTemplatesPreservesUserValues(t *testing.T) {
	cfg := &Config{
		Connectors: ConnectorsConfig{
			GitHub: GitHubConnectorConfig{
				Templates: ConnectorTemplateConfig{
					Name:   "custom-{{ .Title }}",
					Prompt: "custom prompt",
					Tags:   []string{"custom"},
				},
			},
		},
	}
	cfg.applyDefaults()

	assert.Equal(t, "custom-{{ .Title }}", cfg.Connectors.GitHub.Templates.Name)
	assert.Equal(t, "custom prompt", cfg.Connectors.GitHub.Templates.Prompt)
	assert.Equal(t, []string{"custom"}, cfg.Connectors.GitHub.Templates.Tags)
}

func TestConfigLoadsTopLevelConnectors(t *testing.T) {
	yamlSrc := `
connectors:
  github:
    enabled: true
    templates:
      name: "gh-{{ .Fields.number }}"
      prompt: "work on {{ .Title }}"
  external:
    - id: reference
      command: ["hive-reference-connector"]
      templates:
        name: "ref-{{ .ID }}"
        prompt: "do {{ .Title }}"
        tags: ["ref"]
`
	var cfg Config
	require.NoError(t, yaml.Unmarshal([]byte(yamlSrc), &cfg))

	assert.True(t, *cfg.Connectors.GitHub.Enabled)
	assert.Equal(t, "gh-{{ .Fields.number }}", cfg.Connectors.GitHub.Templates.Name)
	require.Len(t, cfg.Connectors.External, 1)
	assert.Equal(t, "reference", cfg.Connectors.External[0].ID)
	assert.Equal(t, []string{"hive-reference-connector"}, cfg.Connectors.External[0].Command)
	assert.Equal(t, "ref-{{ .ID }}", cfg.Connectors.External[0].Templates.Name)
	assert.Equal(t, "do {{ .Title }}", cfg.Connectors.External[0].Templates.Prompt)
	assert.Equal(t, []string{"ref"}, cfg.Connectors.External[0].Templates.Tags)
}

func TestConfigLoadsTopLevelConnectors_WrongNestingIgnored(t *testing.T) {
	yamlSrc := `
plugins:
  connectors:
    external:
      - id: reference
        command: ["hive-reference-connector"]
`
	var cfg Config
	require.NoError(t, yaml.Unmarshal([]byte(yamlSrc), &cfg))

	assert.Empty(t, cfg.Connectors.External)
	assert.Nil(t, cfg.Connectors.GitHub.Enabled)
}

func TestValidateConnectors_ValidGitHubAndExternal(t *testing.T) {
	cfg := validConfig(t)
	cfg.Connectors = ConnectorsConfig{
		GitHub: GitHubConnectorConfig{
			Enabled: boolPtr(true),
			Templates: ConnectorTemplateConfig{
				Name:   "gh-{{ .Fields.number }}-{{ .Title }}",
				Prompt: "Work on {{ .Title }}\n\n{{ .Fields.url }}",
				Tags:   []string{"github", "issue-{{ .Fields.number }}"},
			},
		},
		External: []ExternalConnectorConfig{
			{
				ID:      "reference",
				Command: []string{"hive-reference-connector"},
				Templates: ConnectorTemplateConfig{
					Name:   "ref-{{ .ID }}",
					Prompt: "do {{ .Title }}",
					Tags:   []string{"ref"},
				},
			},
		},
	}

	require.NoError(t, cfg.Validate())
}

func TestValidateConnectors_ExternalMissingID(t *testing.T) {
	cfg := validConfig(t)
	cfg.Connectors.External = []ExternalConnectorConfig{
		{Command: []string{"cmd"}},
	}

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "id")
}

func TestValidateConnectors_ExternalInvalidID(t *testing.T) {
	cfg := validConfig(t)
	cfg.Connectors.External = []ExternalConnectorConfig{
		{ID: "Not_Valid!", Command: []string{"cmd"}},
	}

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid id")
}

func TestValidateConnectors_ExternalDuplicateID(t *testing.T) {
	cfg := validConfig(t)
	cfg.Connectors.External = []ExternalConnectorConfig{
		{ID: "dup", Command: []string{"cmd"}},
		{ID: "dup", Command: []string{"cmd2"}},
	}

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate")
}

func TestValidateConnectors_ExternalMissingCommand(t *testing.T) {
	cfg := validConfig(t)
	cfg.Connectors.External = []ExternalConnectorConfig{
		{ID: "reference"},
	}

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "command")
}

func TestValidateConnectors_DisabledExternalSkipsCommandRequirement(t *testing.T) {
	cfg := validConfig(t)
	cfg.Connectors.External = []ExternalConnectorConfig{
		{ID: "reference", Enabled: boolPtr(false)},
	}

	require.NoError(t, cfg.Validate())
}

func TestValidateConnectors_InvalidTemplateSyntax(t *testing.T) {
	cfg := validConfig(t)
	cfg.Connectors.External = []ExternalConnectorConfig{
		{
			ID:      "reference",
			Command: []string{"cmd"},
			Templates: ConnectorTemplateConfig{
				Name: "{{ .Title",
			},
		},
	}

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "templates")
}

func TestValidateConnectors_MissingFieldsKeyIsNotAConfigError(t *testing.T) {
	// .Fields.<key> is a dynamic, per-item map; referencing an unknown key is
	// only a render-time failure (RenderSessionTemplates), never a config
	// validation error.
	cfg := validConfig(t)
	cfg.Connectors.External = []ExternalConnectorConfig{
		{
			ID:      "reference",
			Command: []string{"cmd"},
			Templates: ConnectorTemplateConfig{
				Name:   "{{ .Fields.whatever }}",
				Prompt: "{{ .Fields.anything }}",
			},
		},
	}

	require.NoError(t, cfg.Validate())
}
