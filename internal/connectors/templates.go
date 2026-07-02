package connectors

import (
	"fmt"

	"github.com/colonyops/hive/pkg/tmpl"
)

// TemplateConfig holds the user-configured Go templates for mapping a
// selected connector item into session creation inputs.
type TemplateConfig struct {
	Name   string
	Prompt string
	Tags   []string
}

// RenderedSession is the result of rendering a TemplateConfig against a
// selected Item, ready to pass into hive.CreateOptions.
type RenderedSession struct {
	Name   string
	Prompt string
	Tags   []string
}

// templateData is the value exposed to connector session templates. Fields
// exposes arbitrary per-item data as a map because template field names are
// dynamic and pkg/tmpl uses missingkey=error.
type templateData struct {
	ID       string
	Title    string
	Subtitle string
	Detail   string
	Fields   map[string]any
}

// RenderSessionTemplates renders cfg's Name, Prompt, and Tags templates
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

	prompt, err := renderer.Render(cfg.Prompt, data)
	if err != nil {
		return RenderedSession{}, fmt.Errorf("prompt template: %w", err)
	}

	tags := make([]string, 0, len(cfg.Tags))
	for i, tagTmpl := range cfg.Tags {
		tag, err := renderer.Render(tagTmpl, data)
		if err != nil {
			return RenderedSession{}, fmt.Errorf("tags[%d] template: %w", i, err)
		}
		tags = append(tags, tag)
	}

	return RenderedSession{
		Name:   name,
		Prompt: prompt,
		Tags:   tags,
	}, nil
}

// detailText returns a plain-text representation of a Detail for use as the
// .Detail template field. Markdown content is used verbatim; KV details are
// not rendered (they have no single plain-text form), and DetailKindNone
// yields an empty string.
func detailText(detail Detail) string {
	if detail.Markdown != nil {
		return detail.Markdown.Content
	}
	return ""
}
