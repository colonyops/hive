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

func TestSourceViewConfigValidate(t *testing.T) {
	tests := []struct {
		name       string
		view       SourceViewConfig
		wantFields []string
	}{
		{
			name:       "empty name",
			view:       SourceViewConfig{Base: "issues", Query: "state:open"},
			wantFields: []string{"source view", "name"},
		},
		{
			name:       "invalid name",
			view:       SourceViewConfig{Name: "bad view", Base: "issues", Query: "state:open"},
			wantFields: []string{"bad view", "name"},
		},
		{
			name:       "issues is reserved",
			view:       SourceViewConfig{Name: "issues", Base: "issues", Query: "state:open"},
			wantFields: []string{"issues", "name", "built-in"},
		},
		{
			name:       "prs is reserved",
			view:       SourceViewConfig{Name: "prs", Base: "prs", Query: "state:open"},
			wantFields: []string{"prs", "name", "built-in"},
		},
		{
			name:       "invalid base",
			view:       SourceViewConfig{Name: "triage", Base: "discussions", Query: "state:open"},
			wantFields: []string{"triage", "base"},
		},
		{
			name:       "empty query",
			view:       SourceViewConfig{Name: "triage", Base: "issues"},
			wantFields: []string{"triage", "query"},
		},
		{
			name:       "whitespace query",
			view:       SourceViewConfig{Name: "triage", Base: "issues", Query: "   "},
			wantFields: []string{"triage", "query"},
		},
		{
			name:       "query newline",
			view:       SourceViewConfig{Name: "triage", Base: "issues", Query: "state:open\nrepo:colonyops/hive"},
			wantFields: []string{"triage", "query", "control character"},
		},
		{
			name:       "query control character",
			view:       SourceViewConfig{Name: "triage", Base: "issues", Query: "state:open\x00"},
			wantFields: []string{"triage", "query", "control character"},
		},
		{
			name:       "malformed scope",
			view:       SourceViewConfig{Name: "triage", Base: "issues", Query: "state:open", Scope: "colonyops"},
			wantFields: []string{"triage", "scope", "owner/repo"},
		},
		{
			name:       "scope with extra segment",
			view:       SourceViewConfig{Name: "triage", Base: "issues", Query: "state:open", Scope: "colonyops/hive/issues"},
			wantFields: []string{"triage", "scope", "owner/repo"},
		},
		{
			name:       "scope control character",
			view:       SourceViewConfig{Name: "triage", Base: "issues", Query: "state:open", Scope: "colonyops/hive\n"},
			wantFields: []string{"triage", "scope", "control character"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.view.Validate()
			require.Error(t, err)
			for _, want := range tt.wantFields {
				assert.Contains(t, err.Error(), want)
			}
		})
	}
}

func TestSourceViewConfigValidate_Valid(t *testing.T) {
	tests := []SourceViewConfig{
		{Name: "my-review_queue", Base: "prs", Query: "review-requested:@me"},
		{Name: "triage", Base: "issues", Query: "label:triage", Scope: "colonyops/hive"},
	}

	for _, view := range tests {
		require.NoError(t, view.Validate())
	}
}

func TestConfigLoadsSourceViewsInDeclarationOrder(t *testing.T) {
	yamlSrc := `
sources:
  views:
    - name: triage
      base: issues
      query: "label:triage"
      scope: colonyops/hive
    - name: alpha
      base: prs
      query: "review-requested:@me"
`
	var cfg Config
	require.NoError(t, yaml.Unmarshal([]byte(yamlSrc), &cfg))
	require.Len(t, cfg.Sources.Views, 2)
	assert.Equal(t, "triage", cfg.Sources.Views[0].Name)
	assert.Equal(t, "alpha", cfg.Sources.Views[1].Name)
	assert.Equal(t, "colonyops/hive", cfg.Sources.Views[0].Scope)
}

func TestValidateSources_DuplicateViewNames(t *testing.T) {
	cfg := validConfig(t)
	cfg.Sources.Views = []SourceViewConfig{
		{Name: "triage", Base: "issues", Query: "label:triage"},
		{Name: "triage", Base: "prs", Query: "review-requested:@me"},
	}

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sources.views[1].name")
	assert.Contains(t, err.Error(), `duplicate source view name "triage"`)
}
