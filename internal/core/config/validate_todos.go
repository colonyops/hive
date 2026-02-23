package config

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/hay-kot/criterio"
)

// builtinSchemes are schemes with built-in action handlers that cannot be
// overridden by user configuration.
var builtinSchemes = map[string]bool{
	"session":     true,
	"review":      true,
	"code-review": true,
	"http":        true,
	"https":       true,
}

// unquotedInterpolation matches template interpolations that do NOT pipe
// through shq. For example: {{ .Value }} or {{ .Foo.Bar }} but NOT
// {{ .Value | shq }} or {{ .Value | shq | other }}.
var unquotedInterpolation = regexp.MustCompile(`\{\{[-\s]*\.[\w.]+\s*\}\}`)

// validateTodos checks the todos.actions configuration:
//   - Normalizes scheme keys to lowercase
//   - Rejects actions for built-in schemes
//   - Validates template syntax
//   - Rejects templates with unquoted interpolations (must use | shq)
func (c *Config) validateTodos() error {
	if len(c.Todos.Actions) == 0 {
		return nil
	}

	var errs criterio.FieldErrorsBuilder
	normalized := make(map[string]string, len(c.Todos.Actions))

	for scheme, tmplStr := range c.Todos.Actions {
		field := fmt.Sprintf("todos.actions[%q]", scheme)
		lower := strings.ToLower(scheme)

		if builtinSchemes[lower] {
			errs = errs.Append(field, fmt.Errorf("cannot override built-in scheme %q", lower))
			continue
		}

		if err := validateTemplate(tmplStr, map[string]any{
			"Scheme": "test",
			"Value":  "test-value",
			"URI":    "test://test-value",
			"Title":  "test todo",
		}); err != nil {
			errs = errs.Append(field, fmt.Errorf("template error: %w", err))
			continue
		}

		if unquotedInterpolation.MatchString(tmplStr) {
			errs = errs.Append(field, fmt.Errorf("template interpolations must use | shq for shell safety"))
			continue
		}

		normalized[lower] = tmplStr
	}

	if err := errs.ToError(); err != nil {
		return err
	}

	// Replace with normalized keys
	c.Todos.Actions = normalized
	return nil
}
