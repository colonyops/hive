// Package tmpl provides template rendering utilities for shell commands.
package tmpl

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

// shellQuote returns a shell-safe quoted string. It wraps the string in single
// quotes and escapes any existing single quotes using the '\" technique.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	// Replace ' with '\'' (end quote, escaped quote, start quote)
	escaped := strings.ReplaceAll(s, "'", `'\''`)
	return "'" + escaped + "'"
}

func stringOrDefault(s, def string) string {
	if s != "" {
		return s
	}
	return def
}

// Config holds all template rendering context.
type Config struct {
	ScriptPaths  map[string]string // "hive-tmux" -> "/path/to/bin/hive-tmux"
	AgentCommand string            // default profile command (e.g., "claude")
	AgentWindow  string            // default profile key / tmux window name
	AgentFlags   string            // shell-quoted flags string
}

func (c Config) scriptPath(name string) string {
	if c.ScriptPaths == nil {
		return name
	}
	if p, ok := c.ScriptPaths[name]; ok {
		return p
	}
	return name
}

// Renderer renders Go templates with shell-oriented helper functions.
type Renderer struct {
	funcs template.FuncMap
}

// New creates a Renderer with the given config baked into template functions.
func New(cfg Config) *Renderer {
	return &Renderer{
		funcs: template.FuncMap{
			"shq":          shellQuote,
			"join":         strings.Join,
			"hiveTmux":     func() string { return cfg.scriptPath("hive-tmux") },
			"agentSend":    func() string { return cfg.scriptPath("agent-send") },
			"agentCommand": func() string { return stringOrDefault(cfg.AgentCommand, "claude") },
			"agentWindow":  func() string { return stringOrDefault(cfg.AgentWindow, "claude") },
			"agentFlags":   func() string { return cfg.AgentFlags },
		},
	}
}

// NewValidation creates a Renderer with safe defaults for template syntax checking.
// Template functions return placeholder values — output is discarded, only parse errors matter.
func NewValidation() *Renderer {
	return New(Config{
		ScriptPaths:  map[string]string{"hive-tmux": "hive-tmux", "agent-send": "agent-send"},
		AgentCommand: "claude",
		AgentWindow:  "claude",
	})
}

// Render executes a Go template string with the given data.
func (r *Renderer) Render(tmpl string, data any) (string, error) {
	t, err := template.New("").Funcs(r.funcs).Option("missingkey=error").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}

	return buf.String(), nil
}
