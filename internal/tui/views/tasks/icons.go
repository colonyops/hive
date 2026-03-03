package tasks

import (
	"github.com/colonyops/hive/internal/core/hc"
	"github.com/colonyops/hive/internal/core/styles"
)

// Status icon characters.
const (
	IconOpen       = "○"
	IconInProgress = "◉"
	IconDone       = " "
	IconCancelled  = ""
	IconExpanded   = ""
	IconCollapsed  = ""
	BadgeBlocked   = "[blocked]"
)

// StatusIcon returns the styled icon string for a status.
func StatusIcon(status hc.Status, blocked bool) string {
	var icon string
	switch status {
	case hc.StatusInProgress:
		icon = styles.TextPrimaryStyle.Render(IconInProgress)
	case hc.StatusDone:
		icon = styles.TextSuccessStyle.Render(IconDone)
	case hc.StatusCancelled:
		icon = styles.TextErrorStyle.Render(IconCancelled)
	default:
		icon = styles.TextMutedStyle.Render(IconOpen)
	}
	if blocked {
		icon += " " + styles.TextWarningStyle.Render(BadgeBlocked)
	}
	return icon
}
