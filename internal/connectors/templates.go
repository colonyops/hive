package connectors

import (
	"fmt"
	"strings"

	"github.com/colonyops/hive/internal/core/session"
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
	// Connector item titles commonly contain punctuation (quotes, "!", "#",
	// parens, etc.) that hive's session name validation
	// (internal/core/session.ValidateName) rejects outright. Sanitize so a
	// template like "gh-{{ .Fields.number }}-{{ .Title }}" always produces a
	// creatable session name instead of surfacing "invalid session name" at
	// CreateSession time, deep past template rendering.
	name = sanitizeSessionName(name, item.ID)

	prompt, err := renderer.Render(cfg.Prompt, data)
	if err != nil {
		return RenderedSession{}, fmt.Errorf("prompt template: %w", err)
	}
	// Trim so templates that reference optional data (e.g. a trailing
	// {{ .Detail }} that rendered empty) don't leave dangling blank lines.
	prompt = strings.TrimSpace(prompt)

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

// sanitizeSessionName rewrites a rendered session Name into kebab-case (via
// session.Slugify) so it passes hive's session name validation and remains
// predictable for issue titles. If nothing usable remains, it falls back to
// the item's ID.
func sanitizeSessionName(name, fallbackID string) string {
	if normalized := session.Slugify(name); normalized != "" {
		return normalized
	}
	if normalized := session.Slugify(fallbackID); normalized != "" {
		return "session-" + normalized
	}
	return "session-item"
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
