package config

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

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
	assert.Empty(t, cfg.Sources.External, "existing configs must not gain external sources")
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

func TestConfigLoadsExternalSourcesInDeclarationOrder(t *testing.T) {
	yamlSrc := `
sources:
  external:
    - name: alerts
      command: [gcx, alerts, --format=json]
      env:
        GCX_PROFILE: production
      timeout: 45s
      templates:
        name: "alert-{{ .ID }}"
        prompt: "Investigate {{ .Title }}"
        tags: [alert, "service-{{ .Fields.service }}"]
    - name: incidents
      command: [incidentctl, list]
`
	var cfg Config
	require.NoError(t, yaml.Unmarshal([]byte(yamlSrc), &cfg))
	require.Len(t, cfg.Sources.External, 2)
	assert.Equal(t, "alerts", cfg.Sources.External[0].Name)
	assert.Equal(t, []string{"gcx", "alerts", "--format=json"}, cfg.Sources.External[0].Command)
	assert.Equal(t, map[string]string{"GCX_PROFILE": "production"}, cfg.Sources.External[0].Env)
	assert.Equal(t, 45*time.Second, cfg.Sources.External[0].Timeout)
	assert.Equal(t, "alert-{{ .ID }}", cfg.Sources.External[0].Templates.Name)
	assert.Equal(t, "incidents", cfg.Sources.External[1].Name)
	assert.Zero(t, cfg.Sources.External[1].Timeout, "zero timeout delegates the default to runtime")
}

func TestConfigLoadsExternalSourcesFromJSONKey(t *testing.T) {
	var cfg Config
	require.NoError(t, json.Unmarshal([]byte(`{
		"sources": {"external": [{"name": "alerts", "command": ["gcx", "alerts"]}]}
	}`), &cfg))

	require.Len(t, cfg.Sources.External, 1)
	assert.Equal(t, "alerts", cfg.Sources.External[0].Name)
	assert.Equal(t, []string{"gcx", "alerts"}, cfg.Sources.External[0].Command)
}

func TestValidateSources_ValidExternalSources(t *testing.T) {
	cfg := validConfig(t)
	cfg.Sources.External = []ExternalSourceConfig{
		{
			Name:    "alerts-prod_2",
			Command: []string{"gcx", "alerts", "--query", "state: firing", "$HOME", "$(not-executed)"},
			Env: map[string]string{
				"GCX_PROFILE": "${PROFILE}",
				"_TRACE":      "enabled",
			},
			Templates: SourceTemplateConfig{
				Name:   "alert-{{ .ID }}",
				Prompt: "Investigate {{ .Title }}: {{ .Detail }}",
				Tags:   []string{"alert", "service-{{ .Fields.service }}"},
			},
		},
		{Name: "incidents", Command: []string{"incidentctl"}, Timeout: maxExternalSourceTimeout},
	}

	require.NoError(t, cfg.Validate())
}

func TestValidateSources_InvalidExternalSources(t *testing.T) {
	validSource := func() ExternalSourceConfig {
		return ExternalSourceConfig{Name: "alerts", Command: []string{"gcx", "alerts"}}
	}

	tests := []struct {
		name      string
		external  []ExternalSourceConfig
		views     []SourceViewConfig
		wantIndex int
		wantName  string
		wantField string
		wantError string
	}{
		{name: "empty name", external: []ExternalSourceConfig{{Command: []string{"gcx"}}}, wantName: "", wantField: "name", wantError: "must not be empty"},
		{name: "whitespace name", external: []ExternalSourceConfig{{Name: "  ", Command: []string{"gcx"}}}, wantName: "  ", wantField: "name", wantError: "must not be empty"},
		{name: "unsafe name", external: []ExternalSourceConfig{{Name: "bad source", Command: []string{"gcx"}}}, wantName: "bad source", wantField: "name", wantError: "alphanumeric"},
		{name: "built-in collision", external: []ExternalSourceConfig{{Name: "issues", Command: []string{"gcx"}}}, wantName: "issues", wantField: "name", wantError: "built-in source"},
		{
			name:      "view collision",
			external:  []ExternalSourceConfig{{Name: "triage", Command: []string{"gcx"}}},
			views:     []SourceViewConfig{{Name: "triage", Base: "issues", Query: "label:triage"}},
			wantName:  "triage",
			wantField: "name",
			wantError: "sources.views[0]",
		},
		{
			name:      "duplicate external name",
			external:  []ExternalSourceConfig{validSource(), validSource()},
			wantIndex: 1,
			wantName:  "alerts",
			wantField: "name",
			wantError: "sources.external[0]",
		},
		{name: "missing argv", external: []ExternalSourceConfig{{Name: "alerts"}}, wantName: "alerts", wantField: "command", wantError: "executable"},
		{name: "empty executable", external: []ExternalSourceConfig{{Name: "alerts", Command: []string{""}}}, wantName: "alerts", wantField: "command[0]", wantError: "must not be empty"},
		{name: "whitespace executable", external: []ExternalSourceConfig{{Name: "alerts", Command: []string{"  "}}}, wantName: "alerts", wantField: "command[0]", wantError: "executable must not be empty"},
		{name: "empty argument", external: []ExternalSourceConfig{{Name: "alerts", Command: []string{"gcx", ""}}}, wantName: "alerts", wantField: "command[1]", wantError: "must not be empty"},
		{name: "control character in argument", external: []ExternalSourceConfig{{Name: "alerts", Command: []string{"gcx", "alerts\nall"}}}, wantName: "alerts", wantField: "command[1]", wantError: "control character"},
		{name: "invalid env name", external: []ExternalSourceConfig{{Name: "alerts", Command: []string{"gcx"}, Env: map[string]string{"2PROFILE": "prod"}}}, wantName: "alerts", wantField: `env["2PROFILE"]`, wantError: "must match"},
		{name: "control character in env value", external: []ExternalSourceConfig{{Name: "alerts", Command: []string{"gcx"}, Env: map[string]string{"PROFILE": "prod\x00secret"}}}, wantName: "alerts", wantField: `env["PROFILE"]`, wantError: "control character"},
		{name: "negative timeout", external: []ExternalSourceConfig{{Name: "alerts", Command: []string{"gcx"}, Timeout: -time.Second}}, wantName: "alerts", wantField: "timeout", wantError: "nonnegative"},
		{name: "excessive timeout", external: []ExternalSourceConfig{{Name: "alerts", Command: []string{"gcx"}, Timeout: maxExternalSourceTimeout + time.Nanosecond}}, wantName: "alerts", wantField: "timeout", wantError: "24h0m0s"},
		{name: "invalid name template", external: []ExternalSourceConfig{{Name: "alerts", Command: []string{"gcx"}, Templates: SourceTemplateConfig{Name: "{{ .Title"}}}, wantName: "alerts", wantField: "templates", wantError: "templates.name"},
		{name: "invalid prompt template", external: []ExternalSourceConfig{{Name: "alerts", Command: []string{"gcx"}, Templates: SourceTemplateConfig{Prompt: "{{ .Detail"}}}, wantName: "alerts", wantField: "templates", wantError: "templates.prompt"},
		{name: "invalid tag template", external: []ExternalSourceConfig{{Name: "alerts", Command: []string{"gcx"}, Templates: SourceTemplateConfig{Tags: []string{"{{ .ID"}}}}, wantName: "alerts", wantField: "templates", wantError: "templates.tags[0]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig(t)
			cfg.Sources.Views = tt.views
			cfg.Sources.External = tt.external

			err := cfg.Validate()
			require.Error(t, err)
			message := err.Error()
			assert.Contains(t, message, fmt.Sprintf("sources.external[%d]", tt.wantIndex))
			assert.Contains(t, message, fmt.Sprintf("external source %q", tt.wantName))
			assert.Contains(t, message, tt.wantField)
			assert.Contains(t, message, tt.wantError)
		})
	}
}

func TestNormalizeSourceCommandName(t *testing.T) {
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
			assert.Equal(t, tt.want, normalizeSourceCommandName(tt.view))
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

func TestSystemCommands_GeneratesExternalSourceCommand(t *testing.T) {
	cfg := &Config{Sources: SourcesConfig{External: []ExternalSourceConfig{{
		Name: "adaptive-alerts", Command: []string{"gcx", "alerts"},
	}}}}

	assert.Equal(t, UserCommand{
		Action: action.TypeOpenSourcePicker,
		Args:   []string{"adaptive-alerts"},
		Scope:  []string{"sessions"},
		Silent: true,
		Help:   "browse adaptive-alerts",
	}, cfg.SystemCommands()["SourceAdaptiveAlerts"])
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

	t.Run("view wins normalized external collision", func(t *testing.T) {
		cfg := &Config{Sources: SourcesConfig{
			Views:    []SourceViewConfig{{Name: "alert-queue", Base: "issues", Query: "label:alert"}},
			External: []ExternalSourceConfig{{Name: "alert_queue", Command: []string{"alerts"}}},
		}}

		command := cfg.SystemCommands()["SourceAlertQueue"]
		assert.Equal(t, []string{"alert-queue"}, command.Args)
		assert.Equal(t, "browse issues matching label:alert", command.Help)
	})

	t.Run("first normalized external source wins", func(t *testing.T) {
		cfg := &Config{Sources: SourcesConfig{External: []ExternalSourceConfig{
			{Name: "adaptive-alerts", Command: []string{"alerts-a"}},
			{Name: "adaptive_alerts", Command: []string{"alerts-b"}},
		}}}

		command := cfg.SystemCommands()["SourceAdaptiveAlerts"]
		assert.Equal(t, []string{"adaptive-alerts"}, command.Args)
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
			warnings := tt.cfg.sourceCommandWarnings()
			require.Len(t, warnings, 1)
			assert.Equal(t, "Sources", warnings[0].Category)
			assert.Equal(t, tt.wantItem, warnings[0].Item)
			for _, message := range tt.wantMessages {
				assert.Contains(t, warnings[0].Message, message)
			}
		})
	}
}

func TestExternalSourceCommandWarnings(t *testing.T) {
	cfg := Config{
		Sources: SourcesConfig{
			Views: []SourceViewConfig{{Name: "alert-queue", Base: "issues", Query: "label:alert"}},
			External: []ExternalSourceConfig{
				{Name: "alert_queue", Command: []string{"alerts"}},
				{Name: "incidents", Command: []string{"incidents"}},
			},
		},
		UserCommands: map[string]UserCommand{"SourceIncidents": {Sh: "custom"}},
	}

	warnings := cfg.sourceCommandWarnings()
	require.Len(t, warnings, 2)
	assert.Equal(t, "alert_queue", warnings[0].Item)
	assert.Contains(t, warnings[0].Message, `external source "alert_queue"`)
	assert.Contains(t, warnings[0].Message, `source view "alert-queue"`)
	assert.Contains(t, warnings[0].Message, `first declared view "alert-queue" wins`)
	assert.Equal(t, "incidents", warnings[1].Item)
	assert.Contains(t, warnings[1].Message, `external source "incidents"`)
	assert.Contains(t, warnings[1].Message, "user command wins")
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

	warnings := cfg.sourceCommandWarnings()
	require.Len(t, warnings, 2)
	assert.Contains(t, warnings[0].Message, "user command wins")
	assert.Contains(t, warnings[1].Message, "first declared view")
}
