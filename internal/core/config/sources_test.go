package config

import (
	"testing"

	"github.com/colonyops/hive/internal/core/action"
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

func TestNormalizeViewCommandName(t *testing.T) {
	tests := []struct {
		name string
		view string
		want string
	}{
		{name: "hyphens", view: "my-review-queue", want: "SourceMyReviewQueue"},
		{name: "underscores", view: "my_review_queue", want: "SourceMyReviewQueue"},
		{name: "spaces", view: "my review queue", want: "SourceMyReviewQueue"},
		{name: "mixed separators", view: "my-review_queue view", want: "SourceMyReviewQueueView"},
		{name: "mixed case", view: "myReviewQueue", want: "SourceMyReviewQueue"},
		{name: "acronym transition", view: "HTTPReviewQueue", want: "SourceHttpReviewQueue"},
		{name: "digits", view: "queue2Review", want: "SourceQueue2Review"},
		{name: "no words", view: "---", want: "SourceView"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, normalizeViewCommandName(tt.view))
		})
	}
}

func TestDefaultUserCommandsCompatibility(t *testing.T) {
	assert.Equal(t, defaultUserCommands, DefaultUserCommands())
}

func TestSystemCommands_GeneratedCommandShape(t *testing.T) {
	cfg := &Config{Sources: SourcesConfig{Views: []SourceViewConfig{{
		Name:  "my-review-queue",
		Base:  "prs",
		Query: "review-requested:@me",
	}}}}

	commands := cfg.SystemCommands()
	assert.Equal(t, UserCommand{
		Action: action.TypeOpenSourcePicker,
		Args:   []string{"my-review-queue"},
		Scope:  []string{"sessions"},
		Silent: true,
		Help:   "browse prs matching review-requested:@me",
	}, commands["SourceMyReviewQueue"])
}

func TestSystemCommands_CollisionResolution(t *testing.T) {
	t.Run("built-in wins", func(t *testing.T) {
		cfg := &Config{Sources: SourcesConfig{Views: []SourceViewConfig{{
			Name:  "Issues",
			Base:  "prs",
			Query: "author:@me",
		}}}}

		assert.Equal(t, defaultUserCommands["SourceIssues"], cfg.SystemCommands()["SourceIssues"])
	})

	t.Run("first normalized view wins", func(t *testing.T) {
		cfg := &Config{Sources: SourcesConfig{Views: []SourceViewConfig{
			{Name: "review-queue", Base: "prs", Query: "review-requested:@me"},
			{Name: "review_queue", Base: "issues", Query: "label:review"},
		}}}

		command := cfg.SystemCommands()["SourceReviewQueue"]
		assert.Equal(t, []string{"review-queue"}, command.Args)
		assert.Equal(t, "browse prs matching review-requested:@me", command.Help)
	})
}

func TestSystemCommands_ReturnsDefensiveBuiltInMap(t *testing.T) {
	cfg := &Config{}
	commands := cfg.SystemCommands()
	delete(commands, "Recycle")

	sourceIssues := commands["SourceIssues"]
	sourceIssues.Args[0] = "changed"
	commands["SourceIssues"] = sourceIssues

	assert.Contains(t, cfg.SystemCommands(), "Recycle")
	assert.Equal(t, []string{"issues"}, defaultUserCommands["SourceIssues"].Args)
}

func TestViewCommandWarnings(t *testing.T) {
	tests := []struct {
		name         string
		cfg          Config
		wantItem     string
		wantMessages []string
	}{
		{
			name: "user command wins",
			cfg: Config{
				Sources: SourcesConfig{Views: []SourceViewConfig{{Name: "triage", Base: "issues", Query: "label:triage"}}},
				UserCommands: map[string]UserCommand{
					"SourceTriage": {Sh: "custom"},
				},
			},
			wantItem:     "triage",
			wantMessages: []string{`generated command "SourceTriage"`, `source view "triage"`, "user command wins"},
		},
		{
			name: "built-in command wins",
			cfg: Config{Sources: SourcesConfig{Views: []SourceViewConfig{{
				Name: "Issues", Base: "issues", Query: "state:open",
			}}}},
			wantItem:     "Issues",
			wantMessages: []string{`generated command "SourceIssues"`, `source view "Issues"`, "built-in command wins"},
		},
		{
			name: "first normalized view wins",
			cfg: Config{Sources: SourcesConfig{Views: []SourceViewConfig{
				{Name: "review-queue", Base: "prs", Query: "review-requested:@me"},
				{Name: "review_queue", Base: "issues", Query: "label:review"},
			}}},
			wantItem:     "review_queue",
			wantMessages: []string{`generated command "SourceReviewQueue"`, `source view "review_queue"`, `source view "review-queue"`, `first declared view "review-queue" wins`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warnings := tt.cfg.viewCommandWarnings()
			require.Len(t, warnings, 1)
			assert.Equal(t, "Sources", warnings[0].Category)
			assert.Equal(t, tt.wantItem, warnings[0].Item)
			for _, message := range tt.wantMessages {
				assert.Contains(t, warnings[0].Message, message)
			}
		})
	}
}

func TestWarningsIncludesViewCommandWarnings(t *testing.T) {
	cfg := Config{
		Sources: SourcesConfig{Views: []SourceViewConfig{{Name: "triage", Base: "issues", Query: "label:triage"}}},
		UserCommands: map[string]UserCommand{
			"SourceTriage": {Sh: "custom"},
		},
	}

	warnings := cfg.Warnings()
	assert.Contains(t, warnings, ValidationWarning{
		Category: "Sources",
		Item:     "triage",
		Message:  `generated command "SourceTriage" for source view "triage" conflicts with a user command; user command wins`,
	})
}

func TestViewCommandWarnings_PrioritizesNormalizationCollisionForDroppedView(t *testing.T) {
	cfg := Config{
		Sources: SourcesConfig{Views: []SourceViewConfig{
			{Name: "review-queue", Base: "prs", Query: "review-requested:@me"},
			{Name: "review_queue", Base: "issues", Query: "label:review"},
		}},
		UserCommands: map[string]UserCommand{"SourceReviewQueue": {Sh: "custom"}},
	}

	warnings := cfg.viewCommandWarnings()
	require.Len(t, warnings, 2)
	assert.Contains(t, warnings[0].Message, "user command wins")
	assert.Contains(t, warnings[1].Message, "first declared view")
}
