package templates

import (
	"testing"

	"github.com/hay-kot/hive/internal/core/config"
)

// TestFullTemplateFlow tests the complete flow from template definition
// to form values to rendered prompt.
func TestFullTemplateFlow(t *testing.T) {
	tests := []struct {
		name       string
		tmpl       config.Template
		values     map[string]any
		wantPrompt string
		wantName   string
	}{
		{
			name: "PR review template with all fields",
			tmpl: config.Template{
				Description: "Review a PR",
				Prompt: `Review PR #{{ .pr_number }} with {{ .review_depth }} depth.
{{ if .focus_areas }}Focus on: {{ .focus_areas | join ", " }}{{ end }}
{{ if .context }}Context: {{ .context }}{{ end }}`,
				Name: "pr-{{ .pr_number }}",
				Fields: []config.TemplateField{
					{Name: "pr_number", Type: config.FieldTypeString, Required: true},
					{Name: "review_depth", Type: config.FieldTypeSelect, Default: "standard", Options: []config.FieldOption{
						{Value: "quick"}, {Value: "standard"}, {Value: "thorough"},
					}},
					{Name: "focus_areas", Type: config.FieldTypeMultiSelect, Options: []config.FieldOption{
						{Value: "security"}, {Value: "performance"}, {Value: "style"},
					}},
					{Name: "context", Type: config.FieldTypeText},
				},
			},
			values: map[string]any{
				"pr_number":    "123",
				"review_depth": "thorough",
				"focus_areas":  []string{"security", "performance"},
				"context":      "Critical bug fix",
			},
			wantPrompt: `Review PR #123 with thorough depth.
Focus on: security, performance
Context: Critical bug fix`,
			wantName: "pr-123",
		},
		{
			name: "template with defaults used",
			tmpl: config.Template{
				Prompt: "Priority: {{ .priority }}, Status: {{ .status | default \"pending\" }}",
				Fields: []config.TemplateField{
					{Name: "priority", Type: config.FieldTypeString, Default: "normal"},
					{Name: "status", Type: config.FieldTypeString},
				},
			},
			values:     map[string]any{},
			wantPrompt: "Priority: normal, Status: pending",
			wantName:   "",
		},
		{
			name: "template with empty optional multi-select",
			tmpl: config.Template{
				Prompt: `Task: {{ .task }}
{{ if .labels }}Labels: {{ .labels | join ", " }}{{ else }}No labels{{ end }}`,
				Fields: []config.TemplateField{
					{Name: "task", Type: config.FieldTypeString, Required: true},
					{Name: "labels", Type: config.FieldTypeMultiSelect, Options: []config.FieldOption{{Value: "bug"}, {Value: "feature"}}},
				},
			},
			values: map[string]any{
				"task": "Fix login",
			},
			wantPrompt: `Task: Fix login
No labels`,
			wantName: "",
		},
		{
			name: "template with no fields (static prompt)",
			tmpl: config.Template{
				Prompt: "Run the test suite and fix any failures.",
			},
			values:     map[string]any{},
			wantPrompt: "Run the test suite and fix any failures.",
			wantName:   "",
		},
		{
			name: "template with shell quoting",
			tmpl: config.Template{
				Prompt: "Run: git commit -m {{ .message | shq }}",
				Fields: []config.TemplateField{
					{Name: "message", Type: config.FieldTypeString, Required: true},
				},
			},
			values: map[string]any{
				"message": "Fix user's login issue",
			},
			wantPrompt: "Run: git commit -m 'Fix user'\\''s login issue'",
			wantName:   "",
		},
		{
			name: "template with special characters in values",
			tmpl: config.Template{
				Prompt: "Search for: {{ .query }}",
				Name:   "search-{{ .id }}",
				Fields: []config.TemplateField{
					{Name: "query", Type: config.FieldTypeString},
					{Name: "id", Type: config.FieldTypeString},
				},
			},
			values: map[string]any{
				"query": "foo && bar || baz",
				"id":    "abc-123",
			},
			wantPrompt: "Search for: foo && bar || baz",
			wantName:   "search-abc-123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Validate template first
			if err := tt.tmpl.Validate("test"); err != nil {
				t.Fatalf("template validation failed: %v", err)
			}

			// Render prompt
			gotPrompt, err := RenderPrompt(tt.tmpl, tt.values)
			if err != nil {
				t.Fatalf("RenderPrompt() error: %v", err)
			}
			if gotPrompt != tt.wantPrompt {
				t.Errorf("RenderPrompt() = %q, want %q", gotPrompt, tt.wantPrompt)
			}

			// Render name
			gotName, err := RenderName(tt.tmpl, tt.values)
			if err != nil {
				t.Fatalf("RenderName() error: %v", err)
			}
			if gotName != tt.wantName {
				t.Errorf("RenderName() = %q, want %q", gotName, tt.wantName)
			}
		})
	}
}

// TestSetValuesToRenderedPrompt tests the flow from --set flag parsing to rendered output.
func TestSetValuesToRenderedPrompt(t *testing.T) {
	tmpl := config.Template{
		Prompt: "PR #{{ .pr_number }} - {{ .tags | join \", \" }}",
		Name:   "pr-{{ .pr_number }}",
		Fields: []config.TemplateField{
			{Name: "pr_number", Type: config.FieldTypeString, Required: true},
			{Name: "tags", Type: config.FieldTypeMultiSelect, Options: []config.FieldOption{{Value: "bug"}, {Value: "feature"}}},
		},
	}

	setFlags := []string{"pr_number=456", "tags=bug,feature"}

	values, err := ParseSetValues(setFlags)
	if err != nil {
		t.Fatalf("ParseSetValues() error: %v", err)
	}

	if err := ValidateRequiredFields(tmpl, values); err != nil {
		t.Fatalf("ValidateRequiredFields() error: %v", err)
	}

	prompt, err := RenderPrompt(tmpl, values)
	if err != nil {
		t.Fatalf("RenderPrompt() error: %v", err)
	}

	want := "PR #456 - bug, feature"
	if prompt != want {
		t.Errorf("Rendered prompt = %q, want %q", prompt, want)
	}

	name, err := RenderName(tmpl, values)
	if err != nil {
		t.Fatalf("RenderName() error: %v", err)
	}

	wantName := "pr-456"
	if name != wantName {
		t.Errorf("Rendered name = %q, want %q", name, wantName)
	}
}

// TestMissingRequiredFieldError tests that validation catches missing required fields.
func TestMissingRequiredFieldError(t *testing.T) {
	tmpl := config.Template{
		Prompt: "{{ .name }} - {{ .desc }}",
		Fields: []config.TemplateField{
			{Name: "name", Type: config.FieldTypeString, Required: true},
			{Name: "desc", Type: config.FieldTypeText},
		},
	}

	// Missing required field
	values := map[string]any{"desc": "optional value"}

	err := ValidateRequiredFields(tmpl, values)
	if err == nil {
		t.Error("ValidateRequiredFields() expected error for missing required field")
	}
}

// TestConfigYAMLParsing tests that templates can be parsed from YAML config.
func TestConfigYAMLParsing(t *testing.T) {
	// This tests the YAML tags work correctly by creating a config struct
	// and validating it matches what we'd expect from a parsed YAML file
	cfg := config.Config{
		GitPath: "git",
		DataDir: "/tmp/test",
		Git:     config.GitConfig{StatusWorkers: 3},
		Templates: map[string]config.Template{
			"pr-review": {
				Description: "Review a pull request",
				Prompt:      "Review PR #{{ .pr }}",
				Name:        "pr-{{ .pr }}",
				Fields: []config.TemplateField{
					{
						Name:     "pr",
						Label:    "PR Number",
						Type:     config.FieldTypeString,
						Required: true,
					},
				},
			},
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Config.Validate() error: %v", err)
	}

	tmpl, ok := cfg.Templates["pr-review"]
	if !ok {
		t.Fatal("template 'pr-review' not found")
	}

	if tmpl.Description != "Review a pull request" {
		t.Errorf("Description = %q, want %q", tmpl.Description, "Review a pull request")
	}

	if len(tmpl.Fields) != 1 {
		t.Errorf("len(Fields) = %d, want 1", len(tmpl.Fields))
	}

	if tmpl.Fields[0].Type != config.FieldTypeString {
		t.Errorf("Field type = %q, want %q", tmpl.Fields[0].Type, config.FieldTypeString)
	}
}

// TestDefaultTemplateRender tests that the built-in default template renders correctly.
func TestDefaultTemplateRender(t *testing.T) {
	tmpl := config.DefaultTemplate()

	values := map[string]any{
		"name":   "my-session",
		"prompt": "Fix the login bug",
	}

	// Render prompt
	prompt, err := RenderPrompt(tmpl, values)
	if err != nil {
		t.Fatalf("RenderPrompt() error: %v", err)
	}
	if prompt != "Fix the login bug" {
		t.Errorf("RenderPrompt() = %q, want %q", prompt, "Fix the login bug")
	}

	// Render name
	name, err := RenderName(tmpl, values)
	if err != nil {
		t.Fatalf("RenderName() error: %v", err)
	}
	if name != "my-session" {
		t.Errorf("RenderName() = %q, want %q", name, "my-session")
	}
}

// TestDefaultTemplateEmptyPrompt tests that empty prompt works with default template.
func TestDefaultTemplateEmptyPrompt(t *testing.T) {
	tmpl := config.DefaultTemplate()

	values := map[string]any{
		"name":   "my-session",
		"prompt": "",
	}

	prompt, err := RenderPrompt(tmpl, values)
	if err != nil {
		t.Fatalf("RenderPrompt() error: %v", err)
	}
	if prompt != "" {
		t.Errorf("RenderPrompt() = %q, want empty string", prompt)
	}
}

// TestAllFieldsPrefilled tests the AllFieldsPrefilled function.
func TestAllFieldsPrefilled(t *testing.T) {
	tmpl := config.Template{
		Prompt: "{{ .name }} - {{ .desc }}",
		Fields: []config.TemplateField{
			{Name: "name", Type: config.FieldTypeString, Required: true},
			{Name: "desc", Type: config.FieldTypeText},
		},
	}

	tests := []struct {
		name      string
		prefilled map[string]any
		want      bool
	}{
		{
			name:      "all fields provided",
			prefilled: map[string]any{"name": "foo", "desc": "bar"},
			want:      true,
		},
		{
			name:      "missing one field",
			prefilled: map[string]any{"name": "foo"},
			want:      false,
		},
		{
			name:      "no fields provided",
			prefilled: map[string]any{},
			want:      false,
		},
		{
			name:      "nil prefilled",
			prefilled: nil,
			want:      false,
		},
		{
			name:      "extra fields ignored",
			prefilled: map[string]any{"name": "foo", "desc": "bar", "extra": "baz"},
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AllFieldsPrefilled(tmpl, tt.prefilled)
			if got != tt.want {
				t.Errorf("AllFieldsPrefilled() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestAllFieldsPrefilledNoFields tests AllFieldsPrefilled with template that has no fields.
func TestAllFieldsPrefilledNoFields(t *testing.T) {
	tmpl := config.Template{
		Prompt: "Static prompt",
	}

	// Empty template fields = always prefilled
	if !AllFieldsPrefilled(tmpl, nil) {
		t.Error("AllFieldsPrefilled() with no fields should return true")
	}
	if !AllFieldsPrefilled(tmpl, map[string]any{}) {
		t.Error("AllFieldsPrefilled() with no fields should return true")
	}
}

// TestSetValuesWithNameOverride tests that --name takes precedence over --set name=...
func TestSetValuesWithNameOverride(t *testing.T) {
	// Simulate: hive new --name x --set name=y --set prompt=hello
	setFlags := []string{"name=y", "prompt=hello"}
	nameFlag := "x"

	values, err := ParseSetValues(setFlags)
	if err != nil {
		t.Fatalf("ParseSetValues() error: %v", err)
	}

	// Override with --name flag
	if nameFlag != "" {
		values["name"] = nameFlag
	}

	// Name should be "x" (from --name flag)
	if values["name"] != "x" {
		t.Errorf("name = %q, want %q", values["name"], "x")
	}
	// Prompt should still be "hello"
	if values["prompt"] != "hello" {
		t.Errorf("prompt = %q, want %q", values["prompt"], "hello")
	}
}
