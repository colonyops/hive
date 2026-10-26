package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func boolPtr(b bool) *bool { return &b }

// TestPartialSourceConfigKeepsDefaults guards the assumption that lets
// sources config skip a post-unmarshal defaults pass: yaml decoding into
// the DefaultConfig-populated struct only touches keys present in the
// document, so a partial templates override keeps the other defaults.
func TestPartialSourceConfigKeepsDefaults(t *testing.T) {
	cfg := DefaultConfig()
	yamlSrc := `
sources:
  issues:
    templates:
      name: "custom-{{ .Title }}"
`
	require.NoError(t, yaml.Unmarshal([]byte(yamlSrc), &cfg))

	assert.Equal(t, "custom-{{ .Title }}", cfg.Sources.Issues.Templates.Name)
	assert.NotEmpty(t, cfg.Sources.Issues.Templates.Prompt, "unset prompt must keep its default")
	assert.NotEmpty(t, cfg.Sources.Issues.Templates.Tags, "unset tags must keep their defaults")
	assert.NotEmpty(t, cfg.Sources.PRs.Templates.Name, "untouched source must keep its defaults")
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
`
	var cfg Config
	require.NoError(t, yaml.Unmarshal([]byte(yamlSrc), &cfg))

	assert.True(t, *cfg.Sources.Issues.Enabled)
	assert.False(t, *cfg.Sources.PRs.Enabled)
	assert.Equal(t, "gh-{{ .Fields.number }}", cfg.Sources.Issues.Templates.Name)
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

func TestValidateSources_ValidHostOverrides(t *testing.T) {
	cfg := validConfig(t)
	cfg.Sources.Hosts = map[string]string{
		"git.acme.com":   "gitea",
		"github.acme.io": "github",
	}

	require.NoError(t, cfg.Validate())
}

func TestValidateSources_InvalidHostBackend(t *testing.T) {
	cfg := validConfig(t)
	cfg.Sources.Hosts = map[string]string{"git.acme.com": "bitbucket"}

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sources.hosts.git.acme.com")
	assert.Contains(t, err.Error(), "invalid backend")
}

func TestValidateSources_InvalidTemplateSyntax(t *testing.T) {
	cfg := validConfig(t)
	cfg.Sources.Issues.Templates.Name = "{{ .Title"

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
