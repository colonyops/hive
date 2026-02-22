package hive

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/todo"
	"github.com/rs/zerolog"
)

const defaultTodoTemplate = `- [ ] **{{ .Title }}** — {{ .Category }} — session: ` + "`{{ .SessionID }}`" + ` — {{ .CreatedAt.Format "2006-01-02 15:04" }}
{{- if .Ref }}
  - ` + "`{{ .Ref }}`" + `
{{- end }}
`

// TodoExporter writes todo items to a markdown file.
type TodoExporter struct {
	cfg    config.TodosExportConfig
	tmpl   *template.Template
	logger zerolog.Logger
}

// NewTodoExporter creates a new exporter from configuration.
func NewTodoExporter(cfg config.TodosExportConfig, logger zerolog.Logger) (*TodoExporter, error) {
	tmplText := defaultTodoTemplate
	if cfg.Template != "" {
		data, err := os.ReadFile(cfg.Template)
		if err != nil {
			return nil, fmt.Errorf("read export template: %w", err)
		}
		tmplText = string(data)
	}

	tmpl, err := template.New("todo").Parse(tmplText)
	if err != nil {
		return nil, fmt.Errorf("parse export template: %w", err)
	}

	return &TodoExporter{
		cfg:    cfg,
		tmpl:   tmpl,
		logger: logger.With().Str("component", "todo-export").Logger(),
	}, nil
}

// RenderTodo renders a single todo item using the template.
func (e *TodoExporter) RenderTodo(t todo.Todo) (string, error) {
	var buf bytes.Buffer
	if err := e.tmpl.Execute(&buf, t); err != nil {
		return "", fmt.Errorf("render todo template: %w", err)
	}
	return buf.String(), nil
}

// Export writes the given todos to the configured path.
func (e *TodoExporter) Export(todos []todo.Todo) error {
	var rendered strings.Builder
	for _, t := range todos {
		s, err := e.RenderTodo(t)
		if err != nil {
			return err
		}
		rendered.WriteString(s)
	}

	content := rendered.String()

	if e.cfg.Markers.Start != "" && e.cfg.Markers.End != "" {
		return e.writeMarkerBounded(content)
	}
	return e.writeAppend(content)
}

// writeAppend appends content to the file, creating it if it doesn't exist.
func (e *TodoExporter) writeAppend(content string) error {
	if err := os.MkdirAll(filepath.Dir(e.cfg.Path), 0o755); err != nil {
		return fmt.Errorf("create export directory: %w", err)
	}

	f, err := os.OpenFile(e.cfg.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open export file: %w", err)
	}
	defer func() { _ = f.Close() }()

	if _, err := f.WriteString(content); err != nil {
		return fmt.Errorf("write export file: %w", err)
	}

	e.logger.Debug().Str("path", e.cfg.Path).Msg("appended todos to export file")
	return nil
}

// writeMarkerBounded replaces content between start/end markers in the file.
func (e *TodoExporter) writeMarkerBounded(content string) error {
	existing, err := os.ReadFile(e.cfg.Path)
	if err != nil {
		return fmt.Errorf("read export file for marker replacement: %w", err)
	}

	text := string(existing)
	startIdx := strings.Index(text, e.cfg.Markers.Start)
	endIdx := strings.Index(text, e.cfg.Markers.End)

	if startIdx == -1 {
		return fmt.Errorf("start marker %q not found in %s", e.cfg.Markers.Start, e.cfg.Path)
	}
	if endIdx == -1 {
		return fmt.Errorf("end marker %q not found in %s", e.cfg.Markers.End, e.cfg.Path)
	}
	if endIdx <= startIdx {
		return fmt.Errorf("end marker appears before start marker in %s", e.cfg.Path)
	}

	// Replace content between markers (after start marker line, before end marker)
	before := text[:startIdx+len(e.cfg.Markers.Start)]
	after := text[endIdx:]
	newContent := before + "\n" + content + after

	// Atomic write: temp file + rename
	tmpPath := e.cfg.Path + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(newContent), 0o644); err != nil {
		return fmt.Errorf("write temp export file: %w", err)
	}
	if err := os.Rename(tmpPath, e.cfg.Path); err != nil {
		_ = os.Remove(tmpPath) // best-effort cleanup
		return fmt.Errorf("rename temp export file: %w", err)
	}

	e.logger.Debug().Str("path", e.cfg.Path).Msg("updated marker-bounded section in export file")
	return nil
}
