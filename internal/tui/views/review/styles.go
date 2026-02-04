package review

import (
	lipgloss "charm.land/lipgloss/v2"
)

// Tokyo Night color palette.
var (
	colorBlue  = lipgloss.Color("#7aa2f7")
	colorGray  = lipgloss.Color("#565f89")
	colorWhite = lipgloss.Color("#c0caf5")
)

// Modal styles for the review view.
var (
	modalStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7aa2f7")).
			Padding(1, 2)

	modalTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#c0caf5"))

	modalHelpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#565f89")).
			MarginTop(1)
)

// Tree characters for rendering document tree.
const (
	treeBranch = "├─"
	treeLast   = "└─"
)
