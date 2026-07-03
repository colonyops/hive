package sourcepicker

import (
	"strings"

	lipgloss "charm.land/lipgloss/v2"

	"github.com/colonyops/hive/internal/core/styles"
	"github.com/colonyops/hive/internal/sources"
	"github.com/colonyops/hive/internal/tui/views/shared"
)

// renderSourceDetail renders a source item's detail pane at the given
// width, dispatching on the detail's kind. Retained for future preview
// pane support (V2).
//
//nolint:unused
func renderSourceDetail(detail sources.Detail, width int) string {
	switch detail.Kind() {
	case sources.DetailKindMarkdown:
		return shared.RenderMarkdown(detail.Markdown.Content, width)
	case sources.DetailKindKV:
		return renderSourceKVDetail(*detail.KV, width)
	case sources.DetailKindNone:
		return styles.TextMutedStyle.Render("(no detail available for this item)")
	default:
		return styles.TextMutedStyle.Render("(no detail available for this item)")
	}
}

//nolint:unused
func renderSourceKVDetail(detail sources.KVDetail, width int) string {
	if len(detail.Sections) == 0 {
		return styles.TextMutedStyle.Render("(no detail available for this item)")
	}

	var sections []string
	for _, section := range detail.Sections {
		sections = append(sections, renderSourceKVSection(section, width))
	}
	return strings.Join(sections, "\n\n")
}

//nolint:unused
func renderSourceKVSection(section sources.KVSection, width int) string {
	var lines []string
	if section.Heading != "" {
		lines = append(lines, styles.TextPrimaryBoldStyle.Render(section.Heading))
	}
	for _, pair := range section.Pairs {
		key := styles.TextMutedStyle.Render(pair.Key + ":")
		lines = append(lines, key+" "+pair.Value)
	}
	return lipgloss.NewStyle().Width(width).Render(strings.Join(lines, "\n"))
}
