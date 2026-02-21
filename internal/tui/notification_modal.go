package tui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/rs/zerolog/log"

	"github.com/colonyops/hive/internal/core/notify"
	"github.com/colonyops/hive/internal/core/styles"
)

const (
	notifyModalWidthPct  = 65
	notifyModalMinWidth  = 80
	notifyModalMaxHeight = 30
	notifyModalMargin    = 4
	notifyModalChrome    = 6 // title + divider + help + spacing
)

// NotificationModal displays a scrollable history of notifications.
type NotificationModal struct {
	store    notify.Store
	viewport viewport.Model
	width    int
	height   int
}

// NewNotificationModal creates a modal showing notification history.
func NewNotificationModal(store notify.Store, width, height int) *NotificationModal {
	modalWidth := calcNotificationModalWidth(width)
	modalHeight := min(height-notifyModalMargin, notifyModalMaxHeight)
	contentHeight := modalHeight - notifyModalChrome

	vp := viewport.New(
		viewport.WithWidth(modalWidth-4), // account for modal padding
		viewport.WithHeight(contentHeight),
	)

	m := &NotificationModal{
		store:    store,
		viewport: vp,
		width:    width,
		height:   height,
	}

	m.refreshContent()
	return m
}

func (m *NotificationModal) refreshContent() {
	if m.store == nil {
		m.viewport.SetContent(styles.TextMutedStyle.Render("No notifications"))
		return
	}

	history, err := m.store.List(context.Background())
	if err != nil {
		log.Error().Err(err).Msg("failed to load notification history")
		m.viewport.SetContent(styles.TextErrorStyle.Render(fmt.Sprintf("failed to load notifications: %v", err)))
		return
	}

	if len(history) == 0 {
		m.viewport.SetContent(styles.TextMutedStyle.Render("No notifications"))
		return
	}

	var b strings.Builder
	for i, n := range history {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(formatNotification(n))
	}

	m.viewport.SetContent(b.String())
}

func formatNotification(n notify.Notification) string {
	ts := styles.TextMutedStyle.Render(n.CreatedAt.Format("15:04:05"))

	var icon string
	var msgStyle lipgloss.Style
	switch n.Level {
	case notify.LevelError:
		icon = styles.IconNotifyError
		msgStyle = styles.TextErrorStyle
	case notify.LevelWarning:
		icon = styles.IconNotifyWarning
		msgStyle = styles.TextWarningStyle
	default:
		icon = styles.IconNotifyInfo
		msgStyle = styles.TextPrimaryStyle
	}

	return fmt.Sprintf("%s %s %s", ts, icon, msgStyle.Render(n.Message))
}

// ScrollUp scrolls the viewport up.
func (m *NotificationModal) ScrollUp() {
	m.viewport.ScrollUp(1)
}

// ScrollDown scrolls the viewport down.
func (m *NotificationModal) ScrollDown() {
	m.viewport.ScrollDown(1)
}

// Clear deletes all notifications and refreshes the view.
func (m *NotificationModal) Clear() error {
	if m.store == nil {
		return nil
	}
	if err := m.store.Clear(context.Background()); err != nil {
		return err
	}
	m.refreshContent()
	return nil
}

// Overlay renders the notification modal centered over the background.
func (m *NotificationModal) Overlay(background string, width, height int) string {
	modalWidth := calcNotificationModalWidth(width)
	modalHeight := min(height-notifyModalMargin, notifyModalMaxHeight)

	scrollInfo := ""
	if m.viewport.TotalLineCount() > m.viewport.VisibleLineCount() {
		scrollInfo = styles.TextMutedStyle.Render(
			fmt.Sprintf(" (%.0f%%)", m.viewport.ScrollPercent()*100),
		)
	}

	divider := styles.TextSurfaceStyle.Render(strings.Repeat("─", modalWidth-6))
	modalContent := lipgloss.JoinVertical(
		lipgloss.Left,
		styles.ModalTitleStyle.Render("Notifications"+scrollInfo),
		divider,
		m.viewport.View(),
		styles.ModalHelpStyle.Render("[j/k] scroll  [D] clear all  [esc] close"),
	)

	modal := styles.ModalStyle.
		Width(modalWidth).
		Height(modalHeight).
		Render(modalContent)

	bgLayer := lipgloss.NewLayer(background)
	modalLayer := lipgloss.NewLayer(modal)

	modalW := lipgloss.Width(modal)
	modalH := lipgloss.Height(modal)
	centerX := max((width-modalW)/2, 0)
	centerY := max((height-modalH)/2, 0)
	modalLayer.X(centerX).Y(centerY).Z(1)

	compositor := lipgloss.NewCompositor(bgLayer, modalLayer)
	return compositor.Render()
}

func calcNotificationModalWidth(termWidth int) int {
	available := max(termWidth-notifyModalMargin, 1)
	target := termWidth * notifyModalWidthPct / 100
	return min(max(target, notifyModalMinWidth), available)
}
