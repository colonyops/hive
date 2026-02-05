package review

import (
	lipgloss "charm.land/lipgloss/v2"
)

// Tokyo Night color palette.
var (
	// Primary colors
	colorBlue  = lipgloss.Color("#7aa2f7")
	colorGray  = lipgloss.Color("#565f89")
	colorWhite = lipgloss.Color("#c0caf5")

	// Background colors
	colorBgDark      = lipgloss.Color("#1a1b26")
	colorBgDarker    = lipgloss.Color("#1f2335")
	colorBgVeryDark  = lipgloss.Color("#282A36")
	colorBgSelection = lipgloss.Color("#3b4261")
	colorBgCursor    = lipgloss.Color("#2a3158")

	// Accent colors
	colorPink        = lipgloss.Color("#f7768e")
	colorPinkVibrant = lipgloss.Color("#F74D50")
	colorGold        = lipgloss.Color("#e0af68")
	colorLightGray   = lipgloss.Color("#9aa5ce")
)

// Modal styles for the review view.
var (
	modalStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBlue).
			Padding(1, 2)

	modalTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorWhite)

	modalHelpStyle = lipgloss.NewStyle().
			Foreground(colorGray).
			MarginTop(1)
)

// Tree characters for rendering document tree.
const (
	treeBranch = "├─"
	treeLast   = "└─"
)
