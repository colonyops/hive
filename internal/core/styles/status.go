package styles

import "github.com/colonyops/hive/internal/core/terminal"

// Status indicator constants for session display.
const (
	StatusIndicatorActive   = "[●]"
	StatusIndicatorApproval = "[!]"
	StatusIndicatorReady    = "[>]"
	StatusIndicatorMissing  = "[?]"
	StatusIndicatorRecycled = "[○]"
)

// RenderStatusIndicator returns a colored status indicator string for the given terminal status.
func RenderStatusIndicator(status terminal.Status) string {
	switch status {
	case terminal.StatusActive:
		return TextSuccessStyle.Render(StatusIndicatorActive)
	case terminal.StatusApproval:
		return TextWarningStyle.Render(StatusIndicatorApproval)
	case terminal.StatusReady:
		return TextSecondaryStyle.Render(StatusIndicatorReady)
	case terminal.StatusMissing:
		return TextMutedStyle.Render(StatusIndicatorMissing)
	default:
		return TextMutedStyle.Render(StatusIndicatorMissing)
	}
}
