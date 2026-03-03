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
	IconExpanded   = ""
	IconCollapsed  = ""
	BadgeBlocked   = "[blocked]"
)

// StatusIcon returns the styled icon string for a status.
func StatusIcon(status hc.Status) string {
	switch status {
	case hc.StatusInProgress:
		return styles.TextPrimaryStyle.Render(IconInProgress)
	case hc.StatusDone:
		return styles.TextSuccessStyle.Render(IconDone)
	case hc.StatusCancelled:
		return styles.TextErrorStyle.Render(IconCancelled)
	default:
		return styles.TextMutedStyle.Render(IconOpen)
	}
}
