package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/colonyops/hive/internal/core/kv"
	"github.com/colonyops/hive/internal/core/styles"
	"github.com/colonyops/hive/internal/tui/jsoncolor"
)

// KVView is a two-column KV browser: searchable key list (left) + colorized JSON preview (right).
type KVView struct {
	keys   []string
	cursor int
	width  int
	height int
	offset int // scroll offset for key list

	// Preview
	previewEntry  *kv.Entry
	previewLines  []string
	previewOffset int // scroll offset for preview pane

	// Filtering
	filtering bool
	filter    string
	filterBuf strings.Builder
	filtered  []int // indices into keys matching filter
}

// NewKVView creates a new KV browser view.
func NewKVView() *KVView {
	return &KVView{
		filtered: make([]int, 0),
	}
}

// SetKeys updates the key list and reapplies the filter.
func (v *KVView) SetKeys(keys []string) {
	v.keys = keys
	v.applyFilter()
	if len(v.filtered) == 0 {
		v.cursor = 0
	} else if v.cursor >= len(v.filtered) {
		v.cursor = len(v.filtered) - 1
	}
	v.clampOffset()
}

// SetPreview sets the preview entry for the selected key.
func (v *KVView) SetPreview(entry *kv.Entry) {
	v.previewEntry = entry
	v.previewOffset = 0
	if entry != nil {
		colorized := jsoncolor.Colorize(entry.Value)
		v.previewLines = strings.Split(colorized, "\n")
	} else {
		v.previewLines = nil
	}
}

// SetSize sets the viewport dimensions.
func (v *KVView) SetSize(width, height int) {
	v.width = width
	v.height = height
	v.clampOffset()
}

// SelectedKey returns the currently selected key, or empty if none.
func (v *KVView) SelectedKey() string {
	if len(v.filtered) == 0 {
		return ""
	}
	return v.keys[v.filtered[v.cursor]]
}

// MoveUp moves the cursor up in the key list.
func (v *KVView) MoveUp() {
	if v.cursor > 0 {
		v.cursor--
		v.clampOffset()
	}
}

// MoveDown moves the cursor down in the key list.
func (v *KVView) MoveDown() {
	if v.cursor < len(v.filtered)-1 {
		v.cursor++
		v.clampOffset()
	}
}

// ScrollPreviewUp scrolls the JSON preview up.
func (v *KVView) ScrollPreviewUp() {
	if v.previewOffset > 0 {
		v.previewOffset--
	}
}

// ScrollPreviewDown scrolls the JSON preview down.
func (v *KVView) ScrollPreviewDown() {
	maxOffset := len(v.previewLines) - v.previewHeight()
	if maxOffset < 0 {
		maxOffset = 0
	}
	if v.previewOffset < maxOffset {
		v.previewOffset++
	}
}

// StartFilter begins filtering mode.
func (v *KVView) StartFilter() {
	v.filtering = true
	v.filterBuf.Reset()
	v.filterBuf.WriteString(v.filter)
}

// IsFiltering returns whether the view is in filter mode.
func (v *KVView) IsFiltering() bool {
	return v.filtering
}

// AddFilterRune adds a character to the filter.
func (v *KVView) AddFilterRune(r rune) {
	v.filterBuf.WriteRune(r)
	v.filter = v.filterBuf.String()
	v.applyFilter()
	v.cursor = 0
	v.offset = 0
}

// DeleteFilterRune removes the last character from the filter.
func (v *KVView) DeleteFilterRune() {
	s := v.filterBuf.String()
	if len(s) > 0 {
		s = s[:len(s)-1]
		v.filterBuf.Reset()
		v.filterBuf.WriteString(s)
		v.filter = s
		v.applyFilter()
		v.cursor = 0
		v.offset = 0
	}
}

// ConfirmFilter exits filtering mode, keeping the filter active.
func (v *KVView) ConfirmFilter() {
	v.filtering = false
}

// CancelFilter clears the filter and exits filtering mode.
func (v *KVView) CancelFilter() {
	v.filtering = false
	v.filter = ""
	v.filterBuf.Reset()
	v.applyFilter()
	v.cursor = 0
	v.offset = 0
}

// View renders the two-column layout.
func (v *KVView) View() string {
	if v.width < 20 || v.height < 3 {
		return ""
	}

	// Layout: key list (20%) | divider (1) | preview (remaining)
	listWidth := int(float64(v.width) * 0.20)
	if listWidth < 15 {
		listWidth = 15
	}
	dividerWidth := 1
	previewWidth := v.width - listWidth - dividerWidth
	if previewWidth < 10 {
		previewWidth = 10
	}

	// Reserve lines for help bar
	contentHeight := v.height - 1
	if contentHeight < 1 {
		contentHeight = 1
	}

	leftPane := v.renderKeyList(listWidth, contentHeight)
	rightPane := v.renderPreview(previewWidth, contentHeight)

	// Vertical divider
	divider := v.renderDivider(contentHeight)

	// Join columns horizontally
	content := joinColumns(leftPane, divider, rightPane, contentHeight)

	// Help bar
	help := v.renderHelp()

	return content + "\n" + help
}

func (v *KVView) renderKeyList(width, height int) []string {
	lines := make([]string, 0, height)

	// Header
	header := styles.TextMutedStyle.Render(truncateOrPad("  Keys", width))
	lines = append(lines, header)

	// Filter line (if active)
	if v.filtering || v.filter != "" {
		filterLine := styles.TextPrimaryStyle.Render("/ ") + v.filter
		if v.filtering {
			filterLine += styles.TextMutedStyle.Render("▎")
		}
		lines = append(lines, truncateOrPad(filterLine, width))
	}

	listHeight := height - len(lines)
	if listHeight < 1 {
		listHeight = 1
	}

	// Keys
	visible := v.filtered
	for i := v.offset; i < len(visible) && i < v.offset+listHeight; i++ {
		key := v.keys[visible[i]]
		var line string
		if i == v.cursor {
			indicator := styles.TextPrimaryStyle.Render("┃ ")
			name := styles.TextForegroundStyle.Render(ansi.Truncate(key, width-3, "…"))
			line = indicator + name
		} else {
			name := styles.TextMutedStyle.Render(ansi.Truncate(key, width-3, "…"))
			line = "  " + name
		}
		lines = append(lines, truncateOrPad(line, width))
	}

	// Pad to fill height
	emptyLine := strings.Repeat(" ", width)
	for len(lines) < height {
		lines = append(lines, emptyLine)
	}

	return lines
}

func (v *KVView) renderPreview(width, height int) []string {
	lines := make([]string, 0, height)
	emptyLine := strings.Repeat(" ", width)
	pad := func(s string) string { return truncateOrPad(s, width) }
	muted := func(s string) string { return pad(styles.TextMutedStyle.Render(s)) }

	if v.previewEntry == nil {
		lines = append(lines, muted("  Preview"))
		lines = append(lines, muted("  No key selected"))
		for len(lines) < height {
			lines = append(lines, emptyLine)
		}
		return lines
	}

	// Line 1: key name (prominent) · created date (muted)
	line1 := "  " + styles.TextPrimaryBoldStyle.Render(v.previewEntry.Key) +
		styles.TextMutedStyle.Render(" · "+v.previewEntry.CreatedAt.Format("2006-01-02 15:04"))
	lines = append(lines, pad(ansi.Truncate(line1, width, "…")))

	// Line 2: divider
	lines = append(lines, muted("  "+strings.Repeat("─", max(width-2, 1))))

	// Line 3: updated · expires (if applicable)
	line3 := "  " + styles.TextMutedStyle.Render("updated ") +
		styles.TextForegroundStyle.Render(v.previewEntry.UpdatedAt.Format("2006-01-02 15:04"))
	if v.previewEntry.ExpiresAt != nil {
		exp := *v.previewEntry.ExpiresAt
		remaining := time.Until(exp)
		var relStr string
		var relRendered string
		if remaining <= 0 {
			relStr = "expired"
			relRendered = styles.TextErrorStyle.Render(relStr)
		} else {
			relStr = formatDuration(remaining)
			relRendered = styles.TextWarningStyle.Render(relStr)
		}
		line3 += styles.TextMutedStyle.Render(" · expires ") +
			styles.TextForegroundStyle.Render(exp.Format("2006-01-02 15:04")) +
			styles.TextMutedStyle.Render(" (") + relRendered + styles.TextMutedStyle.Render(")")
	}
	lines = append(lines, pad(ansi.Truncate(line3, width, "…")))

	// Blank separator before JSON
	lines = append(lines, emptyLine)

	// JSON content
	previewHeight := height - len(lines)
	if previewHeight < 1 {
		previewHeight = 1
	}

	switch {
	case len(v.previewLines) == 0:
		lines = append(lines, muted("  (empty)"))
	default:
		for i := v.previewOffset; i < len(v.previewLines) && i < v.previewOffset+previewHeight; i++ {
			lines = append(lines, pad("  "+v.previewLines[i]))
		}
	}

	// Pad to fill height
	for len(lines) < height {
		lines = append(lines, emptyLine)
	}

	return lines
}

func (v *KVView) renderDivider(height int) []string {
	lines := make([]string, height)
	divChar := styles.TextMutedStyle.Render("│")
	for i := range lines {
		lines[i] = divChar
	}
	return lines
}

func (v *KVView) renderHelp() string {
	return styles.MessagesHelpStyle.Render("↑/↓ navigate • shift+↑/↓ scroll preview • / filter • tab switch view")
}

func (v *KVView) previewHeight() int {
	h := v.height - 2 // header + help
	if h < 1 {
		h = 1
	}
	return h
}

func (v *KVView) visibleLines() int {
	reserved := 2 // header + help
	if v.filtering || v.filter != "" {
		reserved++
	}
	visible := v.height - reserved
	if visible < 1 {
		visible = 1
	}
	return visible
}

func (v *KVView) clampOffset() {
	visible := v.visibleLines()
	total := len(v.filtered)
	if v.cursor < v.offset {
		v.offset = v.cursor
	} else if v.cursor >= v.offset+visible {
		v.offset = v.cursor - visible + 1
	}
	if v.offset > total-visible {
		v.offset = total - visible
	}
	if v.offset < 0 {
		v.offset = 0
	}
}

func (v *KVView) applyFilter() {
	v.filtered = v.filtered[:0]
	lower := strings.ToLower(v.filter)
	for i, key := range v.keys {
		if v.filter == "" || strings.Contains(strings.ToLower(key), lower) {
			v.filtered = append(v.filtered, i)
		}
	}
}

// joinColumns merges line arrays horizontally.
func joinColumns(left, mid, right []string, height int) string {
	var b strings.Builder
	for i := 0; i < height; i++ {
		l, m, r := "", "", ""
		if i < len(left) {
			l = left[i]
		}
		if i < len(mid) {
			m = mid[i]
		}
		if i < len(right) {
			r = right[i]
		}
		b.WriteString(l)
		b.WriteString(m)
		b.WriteString(r)
		if i < height-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// formatDuration returns a compact human-readable duration string.
func formatDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		if m == 0 {
			return fmt.Sprintf("%dh", h)
		}
		return fmt.Sprintf("%dh%dm", h, m)
	default:
		days := int(d.Hours()) / 24
		return fmt.Sprintf("%dd", days)
	}
}

func truncateOrPad(s string, width int) string {
	w := ansi.StringWidth(s)
	if w > width {
		return ansi.Truncate(s, width, "…")
	}
	if w < width {
		return s + strings.Repeat(" ", width-w)
	}
	return s
}
