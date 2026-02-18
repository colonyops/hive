package messages

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/rs/zerolog/log"

	"github.com/colonyops/hive/internal/core/messaging"
	"github.com/colonyops/hive/internal/core/styles"
	"github.com/colonyops/hive/internal/hive"
)

const pollInterval = 500 * time.Millisecond

type messagesLoadedMsg struct {
	messages []messaging.Message
	err      error
}

type pollTickMsg struct{}

// View is the Bubble Tea sub-model for the messages tab.
type View struct {
	ctrl         *Controller
	msgStore     *hive.MessageService
	lastPollTime time.Time
	topicFilter  string
	previewModal *PreviewModal
	copyCommand  string
	width        int
	height       int
	active       bool // true when this view is the active tab
}

// New creates a new messages View.
func New(msgStore *hive.MessageService, topicFilter, copyCommand string) View {
	return View{
		ctrl:        NewController(),
		msgStore:    msgStore,
		topicFilter: topicFilter,
		copyCommand: copyCommand,
	}
}

// Init returns the initial commands for the messages view.
func (v View) Init() tea.Cmd {
	if v.msgStore == nil {
		return nil
	}
	return tea.Batch(
		loadMessages(v.msgStore, v.topicFilter, time.Time{}),
		schedulePollTick(),
	)
}

// Update handles messages for the messages view.
func (v View) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case messagesLoadedMsg:
		return v.handleMessagesLoaded(msg)
	case pollTickMsg:
		return v.handlePollTick()
	case tea.KeyMsg:
		return v.handleKey(msg)
	}
	return v, nil
}

// View renders the messages view.
func (v View) View() string {
	return v.renderList()
}

// HasEditorFocus returns true if the filter input is active.
func (v View) HasEditorFocus() bool {
	return v.ctrl.IsFiltering()
}

// SetSize updates the view dimensions.
func (v *View) SetSize(width, height int) {
	v.width = width
	v.height = height
	v.ctrl.SetSize(v.visibleLines())
}

// SetActive marks whether this view is the currently active tab.
func (v *View) SetActive(active bool) {
	v.active = active
}

// Overlay renders the preview modal over the given background, if active.
func (v View) Overlay(background string, width, height int) string {
	if v.previewModal == nil {
		return background
	}
	return v.previewModal.Overlay(background, width, height)
}

// IsPreviewActive returns true when the preview modal is open.
func (v View) IsPreviewActive() bool {
	return v.previewModal != nil
}

func (v View) handleMessagesLoaded(msg messagesLoadedMsg) (View, tea.Cmd) {
	if msg.err != nil {
		log.Debug().Err(msg.err).Msg("failed to load messages")
		return v, nil
	}
	if len(msg.messages) > 0 {
		v.ctrl.Append(msg.messages)
	}
	v.lastPollTime = time.Now()
	return v, nil
}

func (v View) handlePollTick() (View, tea.Cmd) {
	if v.active && v.msgStore != nil {
		return v, tea.Batch(
			loadMessages(v.msgStore, v.topicFilter, v.lastPollTime),
			schedulePollTick(),
		)
	}
	return v, schedulePollTick()
}

func (v View) handleKey(msg tea.KeyMsg) (View, tea.Cmd) {
	// Preview modal keys take priority
	if v.previewModal != nil {
		return v.handlePreviewKey(msg)
	}

	// Filter mode
	if v.ctrl.IsFiltering() {
		return v.handleFilterKey(msg)
	}

	// Normal mode
	return v.handleNormalKey(msg)
}

func (v View) handlePreviewKey(msg tea.KeyMsg) (View, tea.Cmd) {
	v.previewModal.ClearCopyStatus()

	switch msg.String() {
	case "esc", "enter", "q":
		v.previewModal = nil
		return v, nil
	case "up", "k":
		v.previewModal.ScrollUp()
		return v, nil
	case "down", "j":
		v.previewModal.ScrollDown()
		return v, nil
	case "c", "y":
		if err := v.copyToClipboard(v.previewModal.Payload()); err != nil {
			v.previewModal.SetCopyStatus("Copy failed: " + err.Error())
		} else {
			v.previewModal.SetCopyStatus("Copied!")
		}
		return v, nil
	default:
		v.previewModal.UpdateViewport(msg)
		return v, nil
	}
}

func (v View) handleFilterKey(msg tea.KeyMsg) (View, tea.Cmd) {
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
	return v, nil
}

func (v View) handleNormalKey(msg tea.KeyMsg) (View, tea.Cmd) {
	switch msg.String() {
	case "enter":
		sel := v.ctrl.Selected()
		if sel != nil {
			modal := NewPreviewModal(*sel, v.width, v.height)
			v.previewModal = &modal
		}
	case "up", "k":
		v.ctrl.MoveUp(v.visibleLines())
	case "down", "j":
		v.ctrl.MoveDown(v.visibleLines())
	case "/":
		v.ctrl.StartFilter()
	}
	return v, nil
}

func (v View) copyToClipboard(text string) error {
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

func (v View) visibleLines() int {
	reserved := 2
	if v.ctrl.IsFiltering() || v.ctrl.Filter() != "" {
		reserved++
	}
	visible := v.height - reserved
	if visible < 1 {
		visible = 1
	}
	return visible
}

func (v View) renderList() string {
	var b strings.Builder

	timeWidth := 8
	senderWidth := 14
	topicWidth := 14
	ageWidth := 4
	padding := 5
	contentWidth := v.width - timeWidth - senderWidth - topicWidth - ageWidth - padding - 4
	if contentWidth < 20 {
		contentWidth = 20
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
	timeHeader := fmt.Sprintf("%-*s", timeWidth, "Time")
	senderHeader := fmt.Sprintf("%-*s", senderWidth, "Sender")
	topicHeader := fmt.Sprintf("%-*s", topicWidth, "Topic")
	msgHeader := fmt.Sprintf("%-*s", contentWidth, "Message")
	ageHeader := fmt.Sprintf("%*s", ageWidth, "Age")
	b.WriteString("  ")
	b.WriteString(styles.TextMutedStyle.Render(timeHeader + " " + senderHeader + " " + topicHeader + " " + msgHeader + " " + ageHeader))
	b.WriteString("\n")

	linesRendered := 0
	filteredAt := v.ctrl.FilteredAt()
	displayed := v.ctrl.Displayed()

	if len(filteredAt) == 0 {
		if len(displayed) == 0 {
			b.WriteString(styles.TextMutedStyle.Render("  No messages"))
			b.WriteString("\n")
		} else {
			b.WriteString(styles.TextMutedStyle.Render("  No matching messages"))
			b.WriteString("\n")
		}
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

			line := renderMessageLine(msg, isSelected, timeWidth, senderWidth, topicWidth, contentWidth, ageWidth)
			b.WriteString(line)
			b.WriteString("\n")
			linesRendered++
		}
	}

	visible := v.visibleLines()
	for i := linesRendered; i < visible; i++ {
		b.WriteString("\n")
	}

	b.WriteString(styles.MessagesHelpStyle.Render("↑/↓ navigate • enter preview • / filter • tab switch view"))

	return b.String()
}

func renderMessageLine(msg *messaging.Message, selected bool, _ int, senderW, topicW, contentW, ageW int) string {
	var b strings.Builder

	if selected {
		b.WriteString(styles.TextPrimaryStyle.Render("┃"))
		b.WriteString(" ")
	} else {
		b.WriteString("  ")
	}

	timeStr := msg.CreatedAt.Format("15:04:05")
	b.WriteString(styles.TextMutedStyle.Render(timeStr))
	b.WriteString(" ")

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

	topicColor := styles.ColorForString(msg.Topic)
	topicStyle := lipgloss.NewStyle().Foreground(topicColor)
	topic := msg.Topic
	if len(topic) > topicW-2 {
		topic = topic[:topicW-3] + "…"
	}
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
