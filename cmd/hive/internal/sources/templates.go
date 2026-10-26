package sources

import (
	"fmt"
	"strings"

	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/pkg/tmpl"
)

// TemplateConfig holds the user-configured Go templates for mapping a
// selected source item into session creation inputs.
type TemplateConfig struct {
	Name   string
	Prompt string
	Labels []string
}

// RenderedSession is the result of rendering a TemplateConfig against a
// selected Item, ready to pass into sessionsvc.CreateOptions.
type RenderedSession struct {
	Name   string
	Prompt string
	Labels []string
}

// templateData is the value exposed to source session templates. Fields
// exposes arbitrary per-item data as a map because template field names are
// dynamic and pkg/tmpl uses missingkey=error.
type templateData struct {
	ID       string
	Title    string
	Subtitle string
	Detail   string
	Fields   map[string]any
}

// RenderSessionTemplates renders cfg's Name, Prompt, and Labels templates
// against item and detail, returning the rendered session fields. Each
// template is rendered independently so an error identifies which template
// failed.
func RenderSessionTemplates(cfg TemplateConfig, item Item, detail Detail) (RenderedSession, error) {
	data := templateData{
		ID:       item.ID,
		Title:    item.Title,
		Subtitle: item.Subtitle,
		Detail:   detailText(detail),
		Fields:   item.Fields,
	}

	renderer := tmpl.New(tmpl.Config{})

	name, err := renderer.Render(cfg.Name, data)
	if err != nil {
		return RenderedSession{}, fmt.Errorf("name template: %w", err)
	}
	name = session.ToSessionName(name, "session-"+item.ID)

	prompt, err := renderer.Render(cfg.Prompt, data)
	if err != nil {
		return RenderedSession{}, fmt.Errorf("prompt template: %w", err)
	}
	// Trim so templates that reference optional data (e.g. a trailing
	// {{ .Detail }} that rendered empty) don't leave dangling blank lines.
	prompt = strings.TrimSpace(prompt)

	labels := make([]string, 0, len(cfg.Labels))
	for i, labelTmpl := range cfg.Labels {
		label, err := renderer.Render(labelTmpl, data)
		if err != nil {
			return RenderedSession{}, fmt.Errorf("labels[%d] template: %w", i, err)
		}
		labels = append(labels, label)
	}

	return RenderedSession{
		Name:   name,
		Prompt: prompt,
		Labels: labels,
	}, nil
}

// detailText returns a plain-text representation of a Detail for use as the
// .Detail template field: the markdown content verbatim, or an empty string
// when the item has no detail.
func detailText(detail Detail) string {
	if detail.Markdown != nil {
		return detail.Markdown.Content
	}
	return ""
}
