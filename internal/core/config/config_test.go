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
