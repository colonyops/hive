package messages

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/x/ansi"
	"github.com/rs/zerolog/log"

	"github.com/colonyops/hive/internal/core/messaging"
	"github.com/colonyops/hive/internal/core/styles"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/tui/components"
)

const (
	pollInterval  = 500 * time.Millisecond
	iconDot       = "•"
	unknownSender = "unknown"

	// Minimum width before falling back to single-column layout.
	minDualPaneWidth = 120
)

// Focus pane constants for the split-pane layout.
type focusPane int

const (
	paneList focusPane = iota
	panePreview
)

type messagesLoadedMsg struct {
	messages []messaging.Message
	err      error
}

type pollTickMsg struct{}

var builderPool = sync.Pool{
	New: func() any {
		return &strings.Builder{}
	},
}

// View is the Bubble Tea sub-model for the messages tab.
type View struct {
	ctrl         *Controller
	msgStore     *hive.MessageService
	lastPollTime time.Time
	topicFilter  string
	copyCommand  string
	width        int
	height       int
	active       bool // true when this view is the active tab

	// Split-pane state
	focus      focusPane
	viewport   viewport.Model
	copyStatus string
	splitRatio int // configurable list/preview split percentage

	// Cached selected message index for detecting changes
	lastSelectedIdx int
}

// New creates a new messages View.
func New(msgStore *hive.MessageService, topicFilter, copyCommand string, splitRatio int) *View {
	return &View{
		ctrl:            NewController(),
		msgStore:        msgStore,
		topicFilter:     topicFilter,
		copyCommand:     copyCommand,
		splitRatio:      splitRatio,
		lastSelectedIdx: -1,
	}
}

// Init returns the initial commands for the messages view.
func (v *View) Init() tea.Cmd {
	if v.msgStore == nil {
		return nil
	}
	return tea.Batch(
		loadMessages(v.msgStore, v.topicFilter, time.Time{}),
		schedulePollTick(),
	)
}

// Update handles messages for the messages view.
func (v *View) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case messagesLoadedMsg:
		return v.handleMessagesLoaded(msg)
	case pollTickMsg:
		return v.handlePollTick()
	case tea.KeyPressMsg:
		return v.handleKey(msg)
	}
	return nil
}

// View renders the messages view.
func (v *View) View() string {
	if v.width >= minDualPaneWidth {
		return v.renderDualColumnLayout()
	}
	return v.renderCompactList()
}

// HasEditorFocus returns true if the filter input is active.
func (v *View) HasEditorFocus() bool {
	return v.ctrl.IsFiltering()
}

// HasPreviewFocus returns true when the preview pane has focus.
func (v *View) HasPreviewFocus() bool {
	return v.focus == panePreview
}

// SetSize updates the view dimensions.
func (v *View) SetSize(width, height int) {
	v.width = width
	v.height = height
	v.ctrl.SetSize(v.visibleLines())
	v.sizeViewport()
}

// SetActive marks whether this view is the currently active tab.
func (v *View) SetActive(active bool) {
	v.active = active
}

// --------------------------------------------------------------------
// Key handling
// --------------------------------------------------------------------

func (v *View) handleMessagesLoaded(msg messagesLoadedMsg) tea.Cmd {
	if msg.err != nil {
		log.Error().Err(msg.err).Msg("failed to load messages")
		return nil
	}
	if len(msg.messages) > 0 {
		v.ctrl.Append(msg.messages)
	}
	v.lastPollTime = time.Now()
	return nil
}

func (v *View) handlePollTick() tea.Cmd {
	if v.active && v.msgStore != nil {
		return tea.Batch(
			loadMessages(v.msgStore, v.topicFilter, v.lastPollTime),
			schedulePollTick(),
		)
	}
	return schedulePollTick()
}

func (v *View) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	// Preview pane keys take priority when focused
	if v.focus == panePreview {
		return v.handlePreviewPaneKey(msg)
	}

	// Filter mode
	if v.ctrl.IsFiltering() {
		return v.handleFilterKey(msg)
	}

	// Normal mode (list pane)
	return v.handleNormalKey(msg)
}

func (v *View) handlePreviewPaneKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		v.focus = paneList
		v.copyStatus = ""
	case "up", "k":
		v.viewport.ScrollUp(1)
	case "down", "j":
		v.viewport.ScrollDown(1)
	case "pgup":
		v.viewport.ScrollUp(v.viewport.VisibleLineCount())
	case "pgdown":
		v.viewport.ScrollDown(v.viewport.VisibleLineCount())
	case "c", "y":
		sel := v.ctrl.Selected()
		if sel != nil {
			if err := v.copyToClipboard(sel.Payload); err != nil {
				v.copyStatus = "Copy failed: " + err.Error()
			} else {
				v.copyStatus = "Copied!"
			}
		}
	default:
		v.viewport, _ = v.viewport.Update(msg)
	}
	return nil
}

func (v *View) handleFilterKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		v.ctrl.CancelFilter()
	case "enter":
		v.ctrl.ConfirmFilter()
	case "backspace":
		v.ctrl.DeleteFilterRune()
	default:
		if text := msg.Key().Text; text != "" {
			for _, r := range text {
				v.ctrl.AddFilterRune(r)
			}
		}
	}
	return nil
}

func (v *View) handleNormalKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "enter":
		if v.ctrl.Selected() != nil && v.width >= minDualPaneWidth {
			v.focus = panePreview
			v.copyStatus = ""
			v.updatePreviewContent()
		}
	case "up", "k":
		v.ctrl.MoveUp(v.visibleLines())
	case "down", "j":
		v.ctrl.MoveDown(v.visibleLines())
	case "/":
		v.ctrl.StartFilter()
	case "c", "y":
		sel := v.ctrl.Selected()
		if sel != nil {
			if err := v.copyToClipboard(sel.Payload); err != nil {
				v.copyStatus = "Copy failed: " + err.Error()
			} else {
				v.copyStatus = "Copied!"
			}
		}
	}
	return nil
}

func (v *View) copyToClipboard(text string) error {
	if v.copyCommand == "" {
		return nil
	}
	parts := strings.Fields(v.copyCommand)
	if len(parts) == 0 {
		return nil
	}
	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

// --------------------------------------------------------------------
// Layout helpers
// --------------------------------------------------------------------

func (v *View) visibleLines() int {
	// Both layouts use rule + help bar (2 lines) as footer.
	// Dual-pane also subtracts column header (1 line).
	// Compact subtracts column header (1 line).
	reserved := 3 // rule + help bar + column header
	if v.ctrl.IsFiltering() || v.ctrl.Filter() != "" {
		reserved++
	}
	visible := v.height - reserved
	if visible < 1 {
		visible = 1
	}
	return visible
}

func (v *View) sizeViewport() {
	if v.width < minDualPaneWidth {
		return
	}
	splitPct := v.splitRatio
	if splitPct < 1 || splitPct > 80 {
		splitPct = 25
	}
	listWidth := v.width * splitPct / 100
	if listWidth < 20 {
		listWidth = 20
	}
	previewWidth := v.width - listWidth - 1 // 1 for divider

	// Preview content area: height minus footer (2: rule + help) minus header metadata (3 lines)
	previewHeight := v.height - 2 - 3 // 2 for footer, 3 for preview header
	if previewHeight < 1 {
		previewHeight = 1
	}

	v.viewport = viewport.New(
		viewport.WithWidth(previewWidth-4), // 2 padding each side
		viewport.WithHeight(previewHeight),
	)
	v.lastSelectedIdx = -1 // force re-render
}

// renderDualColumnLayout renders messages list and preview side by side.
func (v *View) renderDualColumnLayout() string {
	// Reserve 2 lines for rule + help bar at bottom
	paneHeight := v.height - 2
	if paneHeight < 1 {
		paneHeight = 1
	}

	// Calculate widths: configurable split, 1 char divider, remaining for preview
	splitPct := v.splitRatio
	if splitPct < 1 || splitPct > 80 {
		splitPct = 25
	}
	listWidth := v.width * splitPct / 100
	if listWidth < 20 {
		listWidth = 20
	}
	dividerWidth := 1
	previewWidth := v.width - listWidth - dividerWidth

	// Render list pane
	listView := v.renderListPane(listWidth, paneHeight)

	// Update preview content if selection changed
	cursor := v.ctrl.Cursor()
	if cursor != v.lastSelectedIdx {
		v.updatePreviewContent()
		v.lastSelectedIdx = cursor
	}

	// Render preview pane
	previewContent := v.renderPreviewPane(previewWidth, paneHeight)

	// Ensure exact dimensions for clean horizontal join
	listView = ensureExactHeight(listView, paneHeight)
	previewContent = ensureExactHeight(previewContent, paneHeight)
	listView = ensureExactWidth(listView, listWidth)
	previewContent = ensureExactWidth(previewContent, previewWidth)

	// Build divider — accent color when preview pane has focus
	dividerStyle := styles.TextMutedStyle
	if v.focus == panePreview {
		dividerStyle = styles.TextPrimaryStyle
	}
	dividerLines := make([]string, paneHeight)
	for i := range dividerLines {
		dividerLines[i] = dividerStyle.Render("│")
	}
	divider := strings.Join(dividerLines, "\n")

	// Dim the list pane when preview has focus
	if v.focus == panePreview {
		listView = styles.TextMutedStyle.Render(listView)
		listView = ensureExactWidth(listView, listWidth)
	}

	panes := lipgloss.JoinHorizontal(lipgloss.Top, listView, divider, previewContent)

	// Common footer: rule + help bar
	bar := components.StatusBar{Width: v.width}
	help := v.dualPaneHelp()

	return panes + "\n" + bar.Rule() + "\n" + bar.Render(help, "")
}

// dualPaneHelp returns context-sensitive help text for the common footer.
func (v *View) dualPaneHelp() string {
	switch {
	case v.copyStatus != "":
		return styles.TextSuccessStyle.Render(v.copyStatus)
	case v.focus == panePreview:
		scrollInfo := ""
		if v.viewport.TotalLineCount() > v.viewport.VisibleLineCount() {
			scrollInfo = fmt.Sprintf(" (%.0f%%)", v.viewport.ScrollPercent()*100)
		}
		return styles.TextMutedStyle.Render(fmt.Sprintf("j/k scroll%s"+components.HelpSep+"c copy"+components.HelpSep+"esc back", scrollInfo))
	default:
		return styles.TextMutedStyle.Render(components.HelpNav + components.HelpSep + components.HelpFilter + components.HelpSep + "enter preview")
	}
}

// renderListPane renders the compact message list for the left pane.
func (v *View) renderListPane(width, maxHeight int) string {
	var b strings.Builder

	ageWidth := 4
	fixed := 2 + 3 + ageWidth // indicator(2) + 3 spaces between columns + age
	remaining := width - fixed
	if remaining < 10 {
		remaining = 10
	}
	senderWidth := remaining / 2
	topicWidth := remaining - senderWidth

	// Filter line
	if v.ctrl.IsFiltering() {
		b.WriteString(" ")
		b.WriteString(styles.TextPrimaryBoldStyle.Render("/ "))
		b.WriteString(v.ctrl.Filter())
		b.WriteString("▎")
		b.WriteString("\n")
	} else if v.ctrl.Filter() != "" {
		b.WriteString(" ")
		b.WriteString(styles.TextMutedStyle.Render(fmt.Sprintf("/ %s", v.ctrl.Filter())))
		b.WriteString("\n")
	}

	// Column headers
	headerLine := fmt.Sprintf("  %-*s %-*s %*s", senderWidth, "Sender", topicWidth, "Topic", ageWidth, "Age")
	if len(headerLine) > width {
		headerLine = headerLine[:width]
	}
	b.WriteString(styles.TextMutedStyle.Render(headerLine))
	b.WriteString("\n")

	linesRendered := 0
	filteredAt := v.ctrl.FilteredAt()
	displayed := v.ctrl.Displayed()

	// Determine available lines for message rows
	reservedLines := 2 // header + help
	if v.ctrl.IsFiltering() || v.ctrl.Filter() != "" {
		reservedLines++
	}
	visibleLines := maxHeight - reservedLines
	if visibleLines < 1 {
		visibleLines = 1
	}

	if len(filteredAt) == 0 {
		if len(displayed) == 0 {
			b.WriteString(styles.TextMutedStyle.Render("  No messages"))
		} else {
			b.WriteString(styles.TextMutedStyle.Render("  No matches"))
		}
		b.WriteString("\n")
		linesRendered = 1
	} else {
		offset := v.ctrl.Offset()
		end := offset + visibleLines
		if end > len(filteredAt) {
			end = len(filteredAt)
		}

		cursor := v.ctrl.Cursor()
		for i := offset; i < end; i++ {
			msgIdx := filteredAt[i]
			msg := &displayed[msgIdx]
			isSelected := i == cursor

			line := v.renderCompactLine(msg, isSelected, senderWidth, topicWidth, ageWidth, width)
			b.WriteString(line)
			b.WriteString("\n")
			linesRendered++
		}
	}

	// Pad remaining lines
	for i := linesRendered; i < visibleLines; i++ {
		b.WriteString("\n")
	}

	return b.String()
}

// renderCompactLine renders a single message row for the compact list.
func (v *View) renderCompactLine(msg *messaging.Message, selected bool, senderW, topicW, ageW, _ int) string {
	var b strings.Builder

	if selected {
		b.WriteString(styles.TextPrimaryStyle.Render("┃"))
		b.WriteString(" ")
	} else {
		b.WriteString("  ")
	}

	sender := msg.Sender
	if sender == "" {
		sender = unknownSender
	}
	if len(sender) > senderW-2 {
		sender = sender[:senderW-3] + "…"
	}
	senderColor := styles.ColorForString(sender)
	senderStyle := lipgloss.NewStyle().Foreground(senderColor)
	b.WriteString(senderStyle.Render(fmt.Sprintf("%-*s", senderW, sender)))
	b.WriteString(" ")

	topic := msg.Topic
	if len(topic) > topicW {
		topic = topic[:topicW-1] + "…"
	}
	topicColor := styles.ColorForString(msg.Topic)
	topicStyle := lipgloss.NewStyle().Foreground(topicColor)
	b.WriteString(topicStyle.Render(fmt.Sprintf("%-*s", topicW, topic)))
	b.WriteString(" ")

	age := formatAge(msg.CreatedAt)
	b.WriteString(styles.TextMutedStyle.Render(fmt.Sprintf("%*s", ageW, age)))

	return b.String()
}

// renderPreviewPane renders the right-hand preview pane.
func (v *View) renderPreviewPane(width, maxHeight int) string {
	sel := v.ctrl.Selected()
	if sel == nil {
		// Empty preview
		emptyMsg := styles.TextMutedStyle.Render("  No message selected")
		return "\n\n" + emptyMsg
	}

	var b strings.Builder
	innerWidth := width - 4 // 2 padding each side

	// Header metadata
	sender := sel.Sender
	if sender == "" {
		sender = unknownSender
	}
	topicStr := styles.PreviewTopicStyle.Render(fmt.Sprintf("[%s]", sel.Topic))
	senderStr := styles.TextSuccessStyle.Render(sender)
	timeStr := styles.TextMutedStyle.Render(sel.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&b, "  %s %s %s %s", topicStr, senderStr, iconDot, timeStr)
	b.WriteString("\n")

	if sel.SessionID != "" {
		sessionStr := styles.PreviewSessionStyle.Render(fmt.Sprintf("  session: %s", sel.SessionID))
		b.WriteString(sessionStr)
		b.WriteString("\n")
	}

	// Divider
	divLen := innerWidth
	if divLen > 40 {
		divLen = 40
	}
	b.WriteString("  ")
	b.WriteString(styles.TextSurfaceStyle.Render(strings.Repeat("─", divLen)))
	b.WriteString("\n")

	// Viewport content with padding
	vpView := v.viewport.View()
	vpLines := strings.Split(vpView, "\n")
	for i, line := range vpLines {
		vpLines[i] = "  " + line
	}
	b.WriteString(strings.Join(vpLines, "\n"))

	// Calculate remaining lines for padding
	headerLines := 3 // topic/sender line + session line + divider
	if sel.SessionID == "" {
		headerLines = 2
	}
	vpHeight := v.viewport.VisibleLineCount()
	usedLines := headerLines + vpHeight
	for i := usedLines; i < maxHeight; i++ {
		b.WriteString("\n")
	}

	return b.String()
}

// updatePreviewContent re-renders the glamour markdown for the selected message.
func (v *View) updatePreviewContent() {
	sel := v.ctrl.Selected()
	if sel == nil {
		v.viewport.SetContent("")
		return
	}

	contentWidth := v.viewport.Width()
	if contentWidth <= 0 {
		contentWidth = 60
	}

	rendered := renderMarkdown(sel.Payload, contentWidth)
	v.viewport.SetContent(rendered)
	v.viewport.SetYOffset(0)
}

// renderCompactList is the fallback single-column layout for narrow terminals.
func (v *View) renderCompactList() string {
	var b strings.Builder

	senderWidth := 14
	topicWidth := 14
	ageWidth := 4
	padding := 5
	contentWidth := v.width - senderWidth - topicWidth - ageWidth - padding - 2
	if contentWidth < 10 {
		contentWidth = 10
	}

	// Filter line
	if v.ctrl.IsFiltering() {
		b.WriteString(" ")
		b.WriteString(styles.TextPrimaryBoldStyle.Render("Filter: "))
		b.WriteString(v.ctrl.Filter())
		b.WriteString("▎")
		b.WriteString("\n")
	} else if v.ctrl.Filter() != "" {
		b.WriteString(" ")
		b.WriteString(styles.TextMutedStyle.Render(fmt.Sprintf("Filter: %s", v.ctrl.Filter())))
		b.WriteString("\n")
	}

	// Column headers
	senderHeader := fmt.Sprintf("%-*s", senderWidth, "Sender")
	topicHeader := fmt.Sprintf("%-*s", topicWidth, "Topic")
	msgHeader := fmt.Sprintf("%-*s", contentWidth, "Message")
	ageHeader := fmt.Sprintf("%*s", ageWidth, "Age")
	b.WriteString("  ")
	b.WriteString(styles.TextMutedStyle.Render(senderHeader + " " + topicHeader + " " + msgHeader + " " + ageHeader))
	b.WriteString("\n")

	linesRendered := 0
	filteredAt := v.ctrl.FilteredAt()
	displayed := v.ctrl.Displayed()

	if len(filteredAt) == 0 {
		if len(displayed) == 0 {
			b.WriteString(styles.TextMutedStyle.Render("  No messages"))
		} else {
			b.WriteString(styles.TextMutedStyle.Render("  No matching messages"))
		}
		b.WriteString("\n")
		linesRendered = 1
	} else {
		visible := v.visibleLines()
		offset := v.ctrl.Offset()
		end := offset + visible
		if end > len(filteredAt) {
			end = len(filteredAt)
		}

		cursor := v.ctrl.Cursor()
		for i := offset; i < end; i++ {
			msgIdx := filteredAt[i]
			msg := &displayed[msgIdx]
			isSelected := i == cursor

			line := renderFallbackLine(msg, isSelected, senderWidth, topicWidth, contentWidth, ageWidth)
			b.WriteString(line)
			b.WriteString("\n")
			linesRendered++
		}
	}

	visible := v.visibleLines()
	for i := linesRendered; i < visible; i++ {
		b.WriteString("\n")
	}

	bar := components.StatusBar{Width: v.width}
	help := styles.TextMutedStyle.Render(components.HelpNav + components.HelpSep + components.HelpFilter)
	b.WriteString(bar.Rule())
	b.WriteString("\n")
	b.WriteString(bar.Render(help, ""))

	return b.String()
}

func renderFallbackLine(msg *messaging.Message, selected bool, senderW, topicW, contentW, ageW int) string {
	var b strings.Builder

	if selected {
		b.WriteString(styles.TextPrimaryStyle.Render("┃"))
		b.WriteString(" ")
	} else {
		b.WriteString("  ")
	}

	sender := msg.Sender
	if sender == "" {
		sender = unknownSender
	}
	if len(sender) > senderW-2 {
		sender = sender[:senderW-3] + "…"
	}
	senderColor := styles.ColorForString(sender)
	senderStyle := lipgloss.NewStyle().Foreground(senderColor)
	senderPadded := fmt.Sprintf("[%-*s]", senderW-2, sender)
	b.WriteString(senderStyle.Render(senderPadded))
	b.WriteString(" ")

	topic := msg.Topic
	if len(topic) > topicW-2 {
		topic = topic[:topicW-3] + "…"
	}
	topicColor := styles.ColorForString(msg.Topic)
	topicStyle := lipgloss.NewStyle().Foreground(topicColor)
	topicPadded := fmt.Sprintf("[%-*s]", topicW-2, topic)
	b.WriteString(topicStyle.Render(topicPadded))
	b.WriteString(" ")

	payload := strings.ReplaceAll(msg.Payload, "\n", " ")
	payload = strings.ReplaceAll(payload, "\t", " ")
	payloadRunes := []rune(payload)
	if len(payloadRunes) > contentW-1 {
		payload = string(payloadRunes[:contentW-1]) + "…"
	}
	payloadStyle := styles.TextForegroundStyle
	if selected {
		payloadStyle = styles.MessagesPayloadSelectedStyle
	}
	payloadPadded := fmt.Sprintf("%-*s", contentW, payload)
	b.WriteString(payloadStyle.Render(payloadPadded))
	b.WriteString(" ")

	age := formatAge(msg.CreatedAt)
	agePadded := fmt.Sprintf("%*s", ageW, age)
	b.WriteString(styles.TextMutedStyle.Render(agePadded))

	return b.String()
}

// --------------------------------------------------------------------
// Markdown rendering
// --------------------------------------------------------------------

func renderMarkdown(payload string, width int) string {
	style := styles.GlamourStyle()
	noMargin := uint(0)
	style.Document.Margin = &noMargin

	renderer, err := glamour.NewTermRenderer(
		glamour.WithStyles(style),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		log.Debug().Err(err).Msg("failed to create markdown renderer, showing raw content")
		return payload
	}

	rendered, err := renderer.Render(payload)
	if err != nil {
		log.Debug().Err(err).Msg("failed to render markdown, showing raw content")
		return payload
	}

	content := strings.TrimSpace(rendered)
	content = stripLeadingDecorative(content)
	content = stripTrailingDecorative(content)
	return content
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func isDecorativeLine(line string) bool {
	stripped := ansiPattern.ReplaceAllString(line, "")
	stripped = strings.TrimSpace(stripped)
	if stripped == "" {
		return true
	}
	for _, r := range stripped {
		if r != '─' && r != '━' && r != '-' && r != '=' {
			return false
		}
	}
	return true
}

func stripLeadingDecorative(content string) string {
	lines := strings.Split(content, "\n")
	start := 0
	for start < len(lines) && isDecorativeLine(lines[start]) {
		start++
	}
	if start > 0 {
		return strings.Join(lines[start:], "\n")
	}
	return content
}

func stripTrailingDecorative(content string) string {
	lines := strings.Split(content, "\n")
	end := len(lines)
	for end > 0 && isDecorativeLine(lines[end-1]) {
		end--
	}
	if end < len(lines) {
		return strings.Join(lines[:end], "\n")
	}
	return content
}

// --------------------------------------------------------------------
// Layout utilities (duplicated from sessions view for now)
// --------------------------------------------------------------------

func ensureExactWidth(content string, width int) string {
	if width <= 0 {
		return content
	}

	lines := strings.Split(content, "\n")
	sb := builderPool.Get().(*strings.Builder)
	defer func() {
		sb.Reset()
		builderPool.Put(sb)
	}()

	for i, line := range lines {
		if i > 0 {
			sb.WriteByte('\n')
		}

		displayWidth := ansi.StringWidth(line)

		switch {
		case displayWidth == width:
			sb.WriteString(line)
		case displayWidth < width:
			sb.WriteString(line)
			sb.WriteString(components.Pad(width - displayWidth))
		default:
			truncated := ansi.TruncateWc(line, width, "")
			sb.WriteString(truncated)
			truncWidth := ansi.StringWidth(truncated)
			if truncWidth < width {
				sb.WriteString(components.Pad(width - truncWidth))
			}
		}
	}

	return sb.String()
}

func ensureExactHeight(content string, n int) string {
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

// --------------------------------------------------------------------
// Time formatting
// --------------------------------------------------------------------

func formatAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// --------------------------------------------------------------------
// Commands
// --------------------------------------------------------------------

func loadMessages(svc *hive.MessageService, topic string, since time.Time) tea.Cmd {
	return func() tea.Msg {
		if svc == nil {
			return messagesLoadedMsg{err: nil}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		messages, err := svc.Subscribe(ctx, topic, since)
		if err != nil {
			if errors.Is(err, messaging.ErrTopicNotFound) {
				return messagesLoadedMsg{messages: nil, err: nil}
			}
			return messagesLoadedMsg{err: err}
		}
		return messagesLoadedMsg{messages: messages, err: nil}
	}
}

func schedulePollTick() tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg {
		return pollTickMsg{}
	})
}
