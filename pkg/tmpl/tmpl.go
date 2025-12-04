// Package tmpl provides template rendering utilities for shell commands.
package tmpl

import (
	"bytes"
	"fmt"
	"text/template"
)

// Render executes a Go template string with the given data.
// Returns an error if the template is invalid or references undefined keys.
func Render(tmpl string, data any) (string, error) {
	t, err := template.New("").Option("missingkey=error").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}

	return buf.String(), nil
}
