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

// scriptPaths holds paths to bundled scripts, set once at startup via SetScriptPaths.
var scriptPaths map[string]string

// agentConfig holds agent profile values, set once at startup via SetAgentConfig.
var agentConfig struct {
	command string
	window  string
	flags   string
}

// SetAgentConfig registers the active agent profile for template functions.
// Call once at startup after config load.
func SetAgentConfig(command, window, flags string) {
	agentConfig.command = command
	agentConfig.window = window
	agentConfig.flags = flags
}

// SetScriptPaths registers bundled script paths for template functions.
// Call once at startup before any templates are rendered.
func SetScriptPaths(paths map[string]string) {
	scriptPaths = paths
}

func scriptPath(name string) string {
	if scriptPaths == nil {
		return name
	}
	if p, ok := scriptPaths[name]; ok {
		return p
	}
	return name
}

var funcs = template.FuncMap{
	"shq":          shellQuote,
	"join":         strings.Join,
	"hiveTmux":     func() string { return scriptPath("hive-tmux") },
	"agentSend":    func() string { return scriptPath("agent-send") },
	"agentCommand": func() string { return stringOrDefault(agentConfig.command, "claude") },
	"agentWindow":  func() string { return stringOrDefault(agentConfig.window, "claude") },
	"agentFlags":   func() string { return agentConfig.flags },
}

func stringOrDefault(s, def string) string {
	if s != "" {
		return s
	}
	return def
}

// Render executes a Go template string with the given data.
// Returns an error if the template is invalid or references undefined keys.
//
// Available template functions:
//   - shq: Shell-quote a string for safe use in shell commands
//   - join: Join string slice with separator (e.g., join .Args " ")
func Render(tmpl string, data any) (string, error) {
	t, err := template.New("").Funcs(funcs).Option("missingkey=error").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}

	return buf.String(), nil
}
