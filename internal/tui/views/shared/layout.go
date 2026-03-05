package shared

import (
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// BuildDividerStyled creates a vertical divider string of the given height with the given style.
func BuildDividerStyled(height int, style lipgloss.Style) string {
	styledChar := style.Render("│")
	lines := make([]string, height)
	for i := range lines {
		lines[i] = styledChar
	}
	return strings.Join(lines, "\n")
}

// PadLines adds left padding to each line of content.
func PadLines(content string, padding int) string {
	pad := strings.Repeat(" ", padding)
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = pad + line
	}
	return strings.Join(lines, "\n")
}

// EnsureExactHeight ensures content has exactly n lines.
func EnsureExactHeight(content string, n int) string {
	if n <= 0 {
		return ""
	}

	lines := strings.Split(content, "\n")

	if len(lines) > n {
		lines = lines[:n]
	} else {
		for len(lines) < n {
			lines = append(lines, "")
		}
	}

	return strings.Join(lines, "\n")
}

// EnsureExactWidth pads or truncates each line to exactly the given width.
func EnsureExactWidth(content string, width int) string {
	if width <= 0 {
		return content
	}

	lines := strings.Split(content, "\n")
	// Pre-allocate: each line ≈ width bytes + newline
	var b strings.Builder
	b.Grow(len(lines) * (width + 1))

	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}

		displayWidth := ansi.StringWidth(line)

		switch {
		case displayWidth == width:
			b.WriteString(line)
		case displayWidth < width:
			b.WriteString(line)
			b.WriteString(strings.Repeat(" ", width-displayWidth))
		default:
			b.WriteString(ansi.Truncate(line, width, ""))
		}
	}

	return b.String()
}
