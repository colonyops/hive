package tui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	"github.com/hay-kot/hive/internal/core/notify"
	"github.com/hay-kot/hive/internal/core/styles"
)

type toastTickMsg time.Time

func scheduleToastTick() tea.Cmd {
	return tea.Tick(toastTickInterval, func(t time.Time) tea.Msg {
		return toastTickMsg(t)
	})
}

// ToastView renders toast notifications and composites them as an overlay.
type ToastView struct {
	controller *ToastController
}

func NewToastView(controller *ToastController) *ToastView {
	return &ToastView{controller: controller}
}

// View renders the toast stack as a single string with toasts stacked
// vertically (oldest at top, newest at bottom).
func (v *ToastView) View() string {
	toasts := v.controller.Toasts()
	if len(toasts) == 0 {
		return ""
	}

	rendered := make([]string, 0, len(toasts))
	for _, t := range toasts {
		rendered = append(rendered, renderToast(t))
	}

	return strings.Join(rendered, "\n")
}

func renderToast(t toast) string {
	var icon string
	var style lipgloss.Style

	switch t.notification.Level {
	case notify.LevelError:
		icon = styles.IconNotifyError
		style = styles.ToastErrorStyle
	case notify.LevelWarning:
		icon = styles.IconNotifyWarning
		style = styles.ToastWarningStyle
	default:
		icon = styles.IconNotifyInfo
		style = styles.ToastInfoStyle
	}

	content := icon + " " + t.notification.Message
	return style.Width(toastWidth).Render(content)
}

// Overlay composites the toast stack over background in the lower-right corner.
func (v *ToastView) Overlay(background string, width, height int) string {
	toastContent := v.View()
	if toastContent == "" {
		return background
	}

	bgLayer := lipgloss.NewLayer(background)
	toastLayer := lipgloss.NewLayer(toastContent)

	toastW := lipgloss.Width(toastContent)
	toastH := lipgloss.Height(toastContent)

	rightX := max(width-toastW-1, 0)
	bottomY := max(height-toastH, 0)

	toastLayer.X(rightX).Y(bottomY).Z(2)

	compositor := lipgloss.NewCompositor(bgLayer, toastLayer)
	return compositor.Render()
}
