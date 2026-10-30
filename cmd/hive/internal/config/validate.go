package config

import (
	"fmt"

	"github.com/colonyops/hive/cmd/hive/internal/sources"
	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/pkg/tmpl"
	"github.com/hay-kot/criterio"
)

// SourceTemplateData defines available fields for source session
// templates (name/prompt/tags). Fields is a map because item field names are
// dynamic per-source; a missing .Fields.<key> is a render-time error, not
// a config-time one.
type SourceTemplateData struct {
	ID       string
	Title    string
	Subtitle string
	Detail   string
	Fields   map[string]any
}

// validateSources checks the top-level sources config template syntax and
// host-backend overrides.
func (c *Config) validateSources() error {
	var errs criterio.FieldErrorsBuilder

	if err := validateSourceTemplateSet("sources.issues.templates", c.Sources.Issues.Templates); err != nil {
		errs = errs.Append("sources.issues.templates", err)
	}
	if err := validateSourceTemplateSet("sources.prs.templates", c.Sources.PRs.Templates); err != nil {
		errs = errs.Append("sources.prs.templates", err)
	}
	for host, backend := range c.Sources.Hosts {
		if _, err := sources.ParseBackend(backend); err != nil {
			errs = errs.Append("sources.hosts."+host, fmt.Errorf("invalid backend %q: expected one of %v", backend, sources.BackendNames()))
		}
	}

	return errs.ToError()
}

// validateSourceTemplateSet syntax-checks the name/prompt/tags templates
// of a single source's SourceTemplateConfig.
func validateSourceTemplateSet(field string, cfg SourceTemplateConfig) error {
	if err := validateSourceTemplate(cfg.Name); err != nil {
		return fmt.Errorf("%s.name: %w", field, err)
	}
	if err := validateSourceTemplate(cfg.Prompt); err != nil {
		return fmt.Errorf("%s.prompt: %w", field, err)
	}
	for i, tag := range cfg.Tags {
		if err := validateSourceTemplate(tag); err != nil {
			return fmt.Errorf("%s.tags[%d]: %w", field, i, err)
		}
	}
	return nil
}

// validateSourceTemplate syntax-checks a single source template.
// Item data (.Fields.<key>) is only known at selection time, so missing
// keys are a render-time concern, not a config error.
func validateSourceTemplate(tmplStr string) error {
	if tmplStr == "" {
		return nil
	}
	return validationRenderer.ValidateSyntax(tmplStr)
}

// ValidateDeep runs the engine's deep validation and the CLI's own: Validate,
// then I/O checks, rule templates and user command templates. configPath is
// the config file to check (empty skips the file check).
func (c *Config) ValidateDeep(configPath string) error {
	if err := c.Validate(); err != nil {
		return err
	}

	return criterio.ValidateStruct(
		c.Config.ValidateDeep(configPath),
		c.validateUserCommandTemplates(),
	)
}

// Warnings returns non-fatal configuration issues in the engine and CLI
// sections.
func (c *Config) Warnings() []config.ValidationWarning {
	var warnings []config.ValidationWarning

	if c.hasLegacyKeybindings {
		warnings = append(warnings, config.ValidationWarning{
			Category: "Keybindings",
			Message:  "top-level 'keybindings' field is deprecated; move entries to 'views.sessions.keybindings'",
		})
	}

	return append(warnings, c.Config.Warnings()...)
}

// validateUserCommandTemplates checks template syntax for usercommand shell commands.
// Basic usercommand structure validation is done by Validate().
func (c *Config) validateUserCommandTemplates() error {
	var errs criterio.FieldErrorsBuilder
	for name, cmd := range c.UserCommands {
		field := fmt.Sprintf("usercommands[%q]", name)
		errs = append(errs, ValidateUserCommandTemplates(field, cmd)...)
	}
	return errs.ToError()
}

// buildValidationData constructs test data for template validation.
// Fixed fields are always present; form fields are added under the Form key.
func buildValidationData(formFields []FormField) map[string]any {
	data := map[string]any{
		"Path":       "/tmp/test",
		"Remote":     "https://github.com/test/repo",
		"ID":         "test123",
		"Name":       "test-session",
		"Tool":       "claude",
		"TmuxWindow": "main",
		"Args":       []string{"arg1", "arg2"},
		"Doc": map[string]any{
			"Path":    "/tmp/test/.hive/plans/test.md",
			"RelPath": ".hive/plans/test.md",
			"Type":    "plan",
		},
	}

	if len(formFields) > 0 {
		formData := make(map[string]any, len(formFields))
		for _, field := range formFields {
			switch {
			case field.Preset != "":
				dummyItem := map[string]any{
					"ID": "test", "Name": "test", "Path": "/tmp", "Remote": "test",
				}
				if field.Multi {
					formData[field.Variable] = []map[string]any{dummyItem}
				} else {
					formData[field.Variable] = dummyItem
				}
			case field.Type == FormTypeMultiSelect:
				formData[field.Variable] = []string{"test"}
			default:
				formData[field.Variable] = "test"
			}
		}
		data["Form"] = formData
	}

	return data
}

// validationRenderer is used for template syntax checking during config validation.
// It uses placeholder values since output is discarded — only parse errors matter.
var validationRenderer = tmpl.NewValidation()

// validateTemplate checks if a template string is valid.
func validateTemplate(tmplStr string, data any) error {
	_, err := validationRenderer.Render(tmplStr, data)
	return err
}
