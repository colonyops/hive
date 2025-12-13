package config

import (
	"testing"
)

func TestTemplateValidation(t *testing.T) {
	tests := []struct {
		name     string
		template Template
		wantErr  string
	}{
		{
			name: "valid template with all field types",
			template: Template{
				Prompt: "Test prompt with {{ .name }}",
				Fields: []TemplateField{
					{Name: "name", Type: FieldTypeString},
					{Name: "description", Type: FieldTypeText},
					{Name: "priority", Type: FieldTypeSelect, Options: []FieldOption{{Value: "low"}, {Value: "high"}}},
					{Name: "tags", Type: FieldTypeMultiSelect, Options: []FieldOption{{Value: "a"}, {Value: "b"}}},
				},
			},
			wantErr: "",
		},
		{
			name: "valid template with no fields",
			template: Template{
				Prompt: "Static prompt with no fields",
			},
			wantErr: "",
		},
		{
			name:     "missing prompt",
			template: Template{},
			wantErr:  "prompt is required",
		},
		{
			name: "missing field name",
			template: Template{
				Prompt: "Test",
				Fields: []TemplateField{{Type: FieldTypeString}},
			},
			wantErr: "field 0: name is required",
		},
		{
			name: "duplicate field names",
			template: Template{
				Prompt: "Test",
				Fields: []TemplateField{
					{Name: "foo", Type: FieldTypeString},
					{Name: "foo", Type: FieldTypeText},
				},
			},
			wantErr: "duplicate field name",
		},
		{
			name: "invalid field type",
			template: Template{
				Prompt: "Test",
				Fields: []TemplateField{
					{Name: "foo", Type: "invalid"},
				},
			},
			wantErr: "invalid type",
		},
		{
			name: "select without options",
			template: Template{
				Prompt: "Test",
				Fields: []TemplateField{
					{Name: "choice", Type: FieldTypeSelect},
				},
			},
			wantErr: "select fields require options",
		},
		{
			name: "multi-select without options",
			template: Template{
				Prompt: "Test",
				Fields: []TemplateField{
					{Name: "choices", Type: FieldTypeMultiSelect},
				},
			},
			wantErr: "select fields require options",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.template.Validate("test")
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("Validate() unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Errorf("Validate() expected error containing %q, got nil", tt.wantErr)
				return
			}
			if !containsSubstring(err.Error(), tt.wantErr) {
				t.Errorf("Validate() error = %q, want error containing %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestFieldTypeIsValid(t *testing.T) {
	tests := []struct {
		fieldType FieldType
		want      bool
	}{
		{FieldTypeString, true},
		{FieldTypeText, true},
		{FieldTypeSelect, true},
		{FieldTypeMultiSelect, true},
		{"", false},
		{"invalid", false},
		{"STRING", false}, // case sensitive
	}

	for _, tt := range tests {
		t.Run(string(tt.fieldType), func(t *testing.T) {
			if got := tt.fieldType.IsValid(); got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestDefaultTemplate(t *testing.T) {
	tmpl := DefaultTemplate()

	// Should have name and prompt fields
	if len(tmpl.Fields) != 2 {
		t.Errorf("DefaultTemplate() fields = %d, want 2", len(tmpl.Fields))
	}

	// Validate passes
	if err := tmpl.Validate("default"); err != nil {
		t.Errorf("DefaultTemplate().Validate() error: %v", err)
	}

	// Check field names
	fieldNames := make(map[string]bool)
	for _, f := range tmpl.Fields {
		fieldNames[f.Name] = true
	}
	if !fieldNames["name"] {
		t.Error("DefaultTemplate() missing 'name' field")
	}
	if !fieldNames["prompt"] {
		t.Error("DefaultTemplate() missing 'prompt' field")
	}

	// Name field should be required
	for _, f := range tmpl.Fields {
		if f.Name == "name" && !f.Required {
			t.Error("DefaultTemplate() 'name' field should be required")
		}
	}
}

func TestMergeTemplates(t *testing.T) {
	tests := []struct {
		name     string
		defaults map[string]Template
		user     map[string]Template
		wantKeys []string
	}{
		{
			name:     "defaults only",
			defaults: map[string]Template{"default": {Prompt: "default prompt"}},
			user:     nil,
			wantKeys: []string{"default"},
		},
		{
			name:     "user only",
			defaults: nil,
			user:     map[string]Template{"custom": {Prompt: "custom prompt"}},
			wantKeys: []string{"custom"},
		},
		{
			name:     "user overrides default",
			defaults: map[string]Template{"default": {Prompt: "default prompt"}},
			user:     map[string]Template{"default": {Prompt: "user prompt"}},
			wantKeys: []string{"default"},
		},
		{
			name:     "user adds to defaults",
			defaults: map[string]Template{"default": {Prompt: "default prompt"}},
			user:     map[string]Template{"custom": {Prompt: "custom prompt"}},
			wantKeys: []string{"default", "custom"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mergeTemplates(tt.defaults, tt.user)
			if len(result) != len(tt.wantKeys) {
				t.Errorf("mergeTemplates() len = %d, want %d", len(result), len(tt.wantKeys))
			}
			for _, key := range tt.wantKeys {
				if _, ok := result[key]; !ok {
					t.Errorf("mergeTemplates() missing key %q", key)
				}
			}
		})
	}

	// Test that user value takes precedence
	defaults := map[string]Template{"default": {Prompt: "default prompt"}}
	user := map[string]Template{"default": {Prompt: "user prompt"}}
	result := mergeTemplates(defaults, user)
	if result["default"].Prompt != "user prompt" {
		t.Errorf("mergeTemplates() default.Prompt = %q, want %q", result["default"].Prompt, "user prompt")
	}
}

func TestLoadDefaultTemplate(t *testing.T) {
	// Create temp dir for test
	tmpDir := t.TempDir()

	// Load with no config file
	cfg, err := Load("", tmpDir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	// Default template should exist
	tmpl, ok := cfg.Templates["default"]
	if !ok {
		t.Fatal("Load() did not include default template")
	}

	// Should match DefaultTemplate()
	defaultTmpl := DefaultTemplate()
	if tmpl.Description != defaultTmpl.Description {
		t.Errorf("default template Description = %q, want %q", tmpl.Description, defaultTmpl.Description)
	}
	if len(tmpl.Fields) != len(defaultTmpl.Fields) {
		t.Errorf("default template Fields len = %d, want %d", len(tmpl.Fields), len(defaultTmpl.Fields))
	}
}
