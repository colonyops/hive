package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"

	"github.com/colonyops/hive/pkg/tmpl"
	"github.com/hay-kot/criterio"
)

// SpawnTemplateData defines available fields for spawn command templates (hive new).
type SpawnTemplateData struct {
	Path       string // Absolute path to the session directory
	Name       string // Session name (directory basename)
	Slug       string // Session slug (URL-safe version of name)
	ContextDir string // Path to context directory
	Owner      string // Repository owner
	Repo       string // Repository name
	Vars       map[string]any
}

// BatchSpawnTemplateData defines available fields for batch_spawn command templates (hive batch).
type BatchSpawnTemplateData struct {
	Path       string // Absolute path to the session directory
	Name       string // Session name (directory basename)
	Prompt     string // User-provided prompt (batch only)
	Slug       string // Session slug (URL-safe version of name)
	ContextDir string // Path to context directory
	Owner      string // Repository owner
	Repo       string // Repository name
	Vars       map[string]any
}

// RecycleTemplateData defines available fields for recycle command templates.
type RecycleTemplateData struct {
	DefaultBranch string // Default branch name (e.g., "main" or "master")
	Vars          map[string]any
}

// ValidationWarning represents a non-fatal configuration issue.
type ValidationWarning struct {
	Category string `json:"category"`
	Item     string `json:"item,omitempty"`
	Message  string `json:"message"`
}

// ValidateDeep performs comprehensive validation of the configuration including
// template syntax, regex patterns, and file accessibility. The configPath argument
// specifies the config file location to validate (empty string skips config file check).
// This calls Validate() first for basic structural validation, then adds I/O checks.
func (c *Config) ValidateDeep(configPath string) error {
	if err := c.Validate(); err != nil {
		return err
	}

	return criterio.ValidateStruct(
		c.validateFileAccess(configPath),
		c.validateVarsFiles(configPath),
		c.validateRules(),
		c.validateUserCommandTemplates(),
	)
}

// Warnings returns non-fatal configuration issues.
func (c *Config) Warnings() []ValidationWarning {
	var warnings []ValidationWarning

	for i, rule := range c.Rules {
		if len(rule.Commands) == 0 && len(rule.Copy) == 0 {
			warnings = append(warnings, ValidationWarning{
				Category: "Rules",
				Item:     fmt.Sprintf("rule %d", i),
				Message:  "rule has neither commands nor copy defined",
			})
		}
	}

	return warnings
}

// validateFileAccess checks config file, data directory, and git executable.
func (c *Config) validateFileAccess(configPath string) error {
	return criterio.ValidateStruct(
		validateConfigFile(configPath),
		criterio.Run("git_path", c.GitPath, gitExecutableExists),
		criterio.Run("data_dir", c.DataDir, isDirectoryOrNotExist),
	)
}

func validateConfigFile(configPath string) error {
	if configPath == "" {
		return nil
	}

	info, err := os.Stat(configPath)
	if os.IsNotExist(err) {
		return nil // not found is fine, using defaults
	}
	if err != nil {
		return criterio.NewFieldErrors("config_file", fmt.Errorf("cannot access: %w", err))
	}
	if info.IsDir() {
		return criterio.NewFieldErrors("config_file", fmt.Errorf("%s is a directory, not a file", configPath))
	}
	return nil
}

// gitExecutableExists validates that the git path is executable.
func gitExecutableExists(path string) error {
	if path == "" {
		return nil
	}
	if _, err := exec.LookPath(path); err != nil {
		return fmt.Errorf("executable not found: %s", path)
	}
	return nil
}

// isDirectoryOrNotExist validates that a path is a directory or doesn't exist.
func isDirectoryOrNotExist(path string) error {
	if path == "" {
		return nil
	}
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil // will be created
	}
	if err != nil {
		return fmt.Errorf("cannot access: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("exists but is not a directory")
	}
	return nil
}

func (c *Config) validateVarsFiles(configPath string) error {
	if len(c.VarsFiles) == 0 {
		return nil
	}

	configDir := filepath.Dir(configPath)
	var errs criterio.FieldErrorsBuilder

	for i, file := range c.VarsFiles {
		path := file
		if !filepath.IsAbs(path) {
			path = filepath.Join(configDir, path)
		}

		if _, err := os.Stat(path); err != nil {
			errs = errs.Append(fmt.Sprintf("vars_files[%d]", i), fmt.Errorf("file not found: %s", file))
		}
	}

	return errs.ToError()
}

// validateRules checks rule patterns are valid regex and command templates are valid.
func (c *Config) validateRules() error {
	var errs criterio.FieldErrorsBuilder
	for i, rule := range c.Rules {
		if rule.Pattern != "" {
			if _, err := regexp.Compile(rule.Pattern); err != nil {
				errs = errs.Append(fmt.Sprintf("rules[%d].pattern", i), fmt.Errorf("invalid regex %q: %w", rule.Pattern, err))
			}
		}

		// Validate spawn templates
		for j, cmd := range rule.Spawn {
			if err := validateTemplate(cmd, SpawnTemplateData{Vars: c.Vars}); err != nil {
				errs = errs.Append(fmt.Sprintf("rules[%d].spawn[%d]", i, j), fmt.Errorf("template error: %w", err))
			}
		}
		for j, cmd := range rule.BatchSpawn {
			if err := validateTemplate(cmd, BatchSpawnTemplateData{Vars: c.Vars}); err != nil {
				errs = errs.Append(fmt.Sprintf("rules[%d].batch_spawn[%d]", i, j), fmt.Errorf("template error: %w", err))
			}
		}
		// Validate recycle templates
		for j, cmd := range rule.Recycle {
			if err := validateTemplate(cmd, RecycleTemplateData{Vars: c.Vars}); err != nil {
				errs = errs.Append(fmt.Sprintf("rules[%d].recycle[%d]", i, j), fmt.Errorf("template error: %w", err))
			}
		}
		// Validate window templates
		for j, w := range rule.Windows {
			prefix := fmt.Sprintf("rules[%d].windows[%d]", i, j)
			if err := validateTemplate(w.Name, BatchSpawnTemplateData{Vars: c.Vars}); err != nil {
				errs = errs.Append(prefix+".name", fmt.Errorf("template error: %w", err))
			}
			if w.Command != "" {
				if err := validateTemplate(w.Command, BatchSpawnTemplateData{Vars: c.Vars}); err != nil {
					errs = errs.Append(prefix+".command", fmt.Errorf("template error: %w", err))
				}
			}
			if w.Dir != "" {
				if err := validateTemplate(w.Dir, BatchSpawnTemplateData{Vars: c.Vars}); err != nil {
					errs = errs.Append(prefix+".dir", fmt.Errorf("template error: %w", err))
				}
			}
		}
	}
	return errs.ToError()
}

// validateUserCommandTemplates checks template syntax for usercommand shell commands.
// Basic usercommand structure validation is done by Validate().
func (c *Config) validateUserCommandTemplates() error {
	var errs criterio.FieldErrorsBuilder
	for name, cmd := range c.UserCommands {
		field := fmt.Sprintf("usercommands[%q]", name)
		testData := buildValidationData(cmd.Form, c.Vars)

		if cmd.Sh != "" {
			if err := validateTemplate(cmd.Sh, testData); err != nil {
				errs = errs.Append(field, fmt.Errorf("template error in sh: %w", err))
			}
		}

		// Validate options templates
		if cmd.Options.SessionName != "" {
			if err := validateTemplate(cmd.Options.SessionName, testData); err != nil {
				errs = errs.Append(field+".options.session_name", err)
			}
		}
		if cmd.Options.Remote != "" {
			if err := validateTemplate(cmd.Options.Remote, testData); err != nil {
				errs = errs.Append(field+".options.remote", err)
			}
		}

		// Validate window templates
		for i, w := range cmd.Windows {
			wfield := fmt.Sprintf("%s.windows[%d]", field, i)
			for tname, tmplStr := range map[string]string{
				".name":    w.Name,
				".command": w.Command,
				".dir":     w.Dir,
			} {
				if tmplStr == "" {
					continue
				}
				if err := validateTemplate(tmplStr, testData); err != nil {
					errs = errs.Append(wfield+tname, err)
				}
			}
		}
	}
	return errs.ToError()
}

// buildValidationData constructs test data for template validation.
// Fixed fields are always present; form fields are added under the Form key.
func buildValidationData(formFields []FormField, vars map[string]any) map[string]any {
	data := map[string]any{
		"Path":       "/tmp/test",
		"Remote":     "https://github.com/test/repo",
		"ID":         "test123",
		"Name":       "test-session",
		"Tool":       "claude",
		"TmuxWindow": "main",
		"Args":       []string{"arg1", "arg2"},
		"Vars":       vars,
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
