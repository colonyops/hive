package connectorpicker

import (
	"strings"

	lipgloss "charm.land/lipgloss/v2"

	"github.com/colonyops/hive/internal/connectors"
	"github.com/colonyops/hive/internal/core/styles"
	"github.com/colonyops/hive/internal/tui/views/shared"
)

// renderConnectorDetail renders a connector item's detail pane at the given
// width, dispatching on the detail's kind. A DetailKindNone detail renders a
// clear placeholder rather than a blank pane.
func renderConnectorDetail(detail connectors.Detail, width int) string {
	switch detail.Kind() {
	case connectors.DetailKindMarkdown:
		return shared.RenderMarkdown(detail.Markdown.Content, width)
	case connectors.DetailKindKV:
		return renderConnectorKVDetail(*detail.KV, width)
	case connectors.DetailKindNone:
		return styles.TextMutedStyle.Render("(no detail available for this item)")
	default:
		return styles.TextMutedStyle.Render("(no detail available for this item)")
	}
}

// renderConnectorKVDetail renders a KVDetail as a heading + key/value sheet
// per section, using the shared modal text styles.
func renderConnectorKVDetail(detail connectors.KVDetail, width int) string {
	if len(detail.Sections) == 0 {
		return styles.TextMutedStyle.Render("(no detail available for this item)")
	}

	var sections []string
	for _, section := range detail.Sections {
		sections = append(sections, renderConnectorKVSection(section, width))
	}
	return strings.Join(sections, "\n\n")
}

// renderConnectorKVSection renders a single KVSection: an optional heading
// followed by "key: value" rows.
func renderConnectorKVSection(section connectors.KVSection, width int) string {
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
