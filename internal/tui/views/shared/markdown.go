package shared

import (
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/rs/zerolog/log"

	"github.com/colonyops/hive/internal/core/styles"
)

// cachedRenderer holds a reusable glamour TermRenderer keyed by word-wrap width.
var (
	cachedRenderer      *glamour.TermRenderer
	cachedRendererWidth int
)

// GetMarkdownRenderer returns a glamour TermRenderer, reusing a cached instance
// when the width hasn't changed.
func GetMarkdownRenderer(width int) (*glamour.TermRenderer, error) {
	if cachedRenderer != nil && cachedRendererWidth == width {
		return cachedRenderer, nil
	}

	style := styles.GlamourStyle()
	noMargin := uint(0)
	style.Document.Margin = &noMargin

	r, err := glamour.NewTermRenderer(
		glamour.WithStyles(style),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return nil, err
	}

	cachedRenderer = r
	cachedRendererWidth = width
	return r, nil
}

// RenderMarkdown renders text as styled markdown using glamour.
// Glamour can panic on certain inputs, so we recover gracefully.
func RenderMarkdown(text string, width int) (result string) {
	defer func() {
		if r := recover(); r != nil {
			log.Warn().Interface("panic", r).Msg("shared: glamour panicked during render")
			result = text
		}
	}()

	r, err := GetMarkdownRenderer(width)
	if err != nil {
		log.Debug().Err(err).Msg("shared: failed to create markdown renderer")
		return text
	}

	rendered, err := r.Render(text)
	if err != nil {
		log.Debug().Err(err).Msg("shared: failed to render markdown")
		return text
	}

	return strings.TrimSpace(rendered)
}
