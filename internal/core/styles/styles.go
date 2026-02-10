// Package styles provides shared lipgloss v2 styles for CLI and TUI components.
package styles

import (
	"image/color"

	lipgloss "charm.land/lipgloss/v2"
)

// CurrentPalette holds the active theme palette.
var CurrentPalette Palette

// Semantic color aliases set by SetTheme.
var (
	ColorPrimary    color.Color
	ColorSecondary  color.Color
	ColorForeground color.Color
	ColorMuted      color.Color
	ColorBackground color.Color
	ColorSurface    color.Color
	ColorSuccess    color.Color
	ColorWarning    color.Color
	ColorError      color.Color
)

// Style exports.
var (
	// CLI styles.
	CommandHeaderStyle lipgloss.Style

	// TUI shared styles.
	ModalStyle               lipgloss.Style
	ModalTitleStyle          lipgloss.Style
	ModalHelpStyle           lipgloss.Style
	ModalButtonStyle         lipgloss.Style
	ModalButtonSelectedStyle lipgloss.Style
	TextMutedStyle           lipgloss.Style
	TextPrimaryStyle         lipgloss.Style
	TextPrimaryBoldStyle     lipgloss.Style
	TextSecondaryStyle       lipgloss.Style
	TextForegroundStyle      lipgloss.Style
	TextForegroundBoldStyle  lipgloss.Style
	TextSuccessStyle         lipgloss.Style
	TextWarningStyle         lipgloss.Style
	TextErrorStyle           lipgloss.Style
	TextSurfaceStyle         lipgloss.Style

	ViewSelectedStyle lipgloss.Style
	ViewNormalStyle   lipgloss.Style

	FormTitleStyle        lipgloss.Style
	FormFieldStyle        lipgloss.Style
	FormFieldFocusedStyle lipgloss.Style

	// Help dialog styles.
	HelpDialogSectionStyle lipgloss.Style
	HelpDialogHelpStyle    lipgloss.Style
	HelpDialogModalStyle   lipgloss.Style

	// Confirm modal styles.
	ConfirmMessageStyle lipgloss.Style

	// Review comment modal styles.
	ReviewCommentTitleStyle   lipgloss.Style
	ReviewCommentLabelStyle   lipgloss.Style
	ReviewCommentContextStyle lipgloss.Style
	ReviewCommentHelpStyle    lipgloss.Style

	// Review finalize modal styles.
	ReviewFinalizeOptionStyle lipgloss.Style
	ReviewFinalizeModalStyle  lipgloss.Style

	// Review view styles.
	ReviewEmptyMessageStyle       lipgloss.Style
	ReviewOverlayModalStyle       lipgloss.Style
	ReviewSelectionStyle          lipgloss.Style
	ReviewCursorStyle             lipgloss.Style
	ReviewSearchMatchStyle        lipgloss.Style
	ReviewCurrentSearchMatchStyle lipgloss.Style
	ReviewCommentedLineNumStyle   lipgloss.Style
	ReviewSearchInputStyle        lipgloss.Style
	ReviewModeNormalStyle         lipgloss.Style
	ReviewModeVisualStyle         lipgloss.Style
	ReviewModeSearchStyle         lipgloss.Style
	ReviewPosStyle                lipgloss.Style
	ReviewHelpStyle               lipgloss.Style
	ReviewStatusBarBgStyle        lipgloss.Style
	ReviewInlineCommentStyle      lipgloss.Style

	ReviewTreeHeaderStyle         lipgloss.Style
	ReviewTreeHeaderSelectedStyle lipgloss.Style

	// Messages view styles.
	MessagesHelpStyle            lipgloss.Style
	MessagesPayloadSelectedStyle lipgloss.Style

	// Command palette styles.
	CommandPaletteHelpSelectedStyle lipgloss.Style
	CommandPaletteMoreStyle         lipgloss.Style

	// Select field styles.
	SelectFieldItemSelectedStyle lipgloss.Style

	// TUI layout styles.
	ListFilterPromptStyle  lipgloss.Style
	ListHelpContainerStyle lipgloss.Style
	TabBrandingStyle       lipgloss.Style
	PreviewHeaderNameStyle lipgloss.Style
	PreviewTopicStyle      lipgloss.Style
	PreviewSessionStyle    lipgloss.Style
)

// ColorPool is used for deterministic color hashing of topics and senders.
var ColorPool []color.Color

// SetTheme sets the active palette and rebuilds all global styles.
func SetTheme(p Palette) {
	CurrentPalette = p

	ColorPrimary = p.Primary
	ColorSecondary = p.Secondary
	ColorForeground = p.Foreground
	ColorMuted = p.Muted
	ColorBackground = p.Background
	ColorSurface = p.Surface
	ColorSuccess = p.Success
	ColorWarning = p.Warning
	ColorError = p.Error

	CommandHeaderStyle = lipgloss.NewStyle().
		Foreground(ColorPrimary).
		Bold(true)

	ModalStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorPrimary).
		Padding(1, 2)
	ModalTitleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorForeground)
	ModalHelpStyle = lipgloss.NewStyle().
		Foreground(ColorMuted).
		MarginTop(1)
	ModalButtonStyle = lipgloss.NewStyle().
		Padding(0, 1).
		Background(ColorSurface).
		Foreground(ColorMuted)
	ModalButtonSelectedStyle = lipgloss.NewStyle().
		Padding(0, 1).
		Background(ColorPrimary).
		Foreground(ColorBackground).
		Bold(true)

	TextMutedStyle = lipgloss.NewStyle().
		Foreground(ColorMuted)
	TextPrimaryStyle = lipgloss.NewStyle().
		Foreground(ColorPrimary)
	TextPrimaryBoldStyle = lipgloss.NewStyle().
		Foreground(ColorPrimary).
		Bold(true)
	TextSecondaryStyle = lipgloss.NewStyle().
		Foreground(ColorSecondary)
	TextForegroundStyle = lipgloss.NewStyle().
		Foreground(ColorForeground)
	TextForegroundBoldStyle = lipgloss.NewStyle().
		Foreground(ColorForeground).
		Bold(true)
	TextSuccessStyle = lipgloss.NewStyle().
		Foreground(ColorSuccess)
	TextWarningStyle = lipgloss.NewStyle().
		Foreground(ColorWarning)
	TextErrorStyle = lipgloss.NewStyle().
		Foreground(ColorError)
	TextSurfaceStyle = lipgloss.NewStyle().
		Foreground(ColorSurface)

	ViewSelectedStyle = lipgloss.NewStyle().
		Foreground(ColorPrimary).
		Bold(true)
	ViewNormalStyle = lipgloss.NewStyle().
		Foreground(ColorMuted)

	FormTitleStyle = lipgloss.NewStyle().
		Foreground(ColorPrimary).
		Bold(true)
	FormFieldStyle = lipgloss.NewStyle().
		Border(lipgloss.ThickBorder(), false, false, false, true).
		BorderForeground(ColorMuted).
		PaddingLeft(1)
	FormFieldFocusedStyle = lipgloss.NewStyle().
		Border(lipgloss.ThickBorder(), false, false, false, true).
		BorderForeground(ColorPrimary).
		PaddingLeft(1)

	HelpDialogSectionStyle = lipgloss.NewStyle().
		Foreground(ColorMuted).
		Italic(true)
	HelpDialogHelpStyle = lipgloss.NewStyle().
		Foreground(ColorMuted).
		MarginTop(1)
	HelpDialogModalStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorPrimary).
		Padding(1, 2)

	ConfirmMessageStyle = lipgloss.NewStyle().
		Foreground(ColorForeground).
		MarginBottom(1)

	ReviewCommentTitleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary).
		MarginBottom(1)
	ReviewCommentLabelStyle = lipgloss.NewStyle().
		Foreground(ColorMuted).
		MarginBottom(1)
	ReviewCommentContextStyle = lipgloss.NewStyle().
		Foreground(ColorMuted).
		Italic(true).
		MarginBottom(1)
	ReviewCommentHelpStyle = lipgloss.NewStyle().
		Foreground(ColorMuted).
		MarginTop(1)

	ReviewFinalizeOptionStyle = lipgloss.NewStyle().
		Foreground(ColorPrimary).
		Bold(true)
	ReviewFinalizeModalStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorPrimary).
		Padding(1, 2).
		Width(60).
		Background(ColorBackground)

	ReviewEmptyMessageStyle = lipgloss.NewStyle().
		Foreground(ColorForeground).
		Padding(2, 4)
	ReviewOverlayModalStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorPrimary).
		Padding(1, 2).
		Background(ColorBackground)
	ReviewSelectionStyle = lipgloss.NewStyle().
		Background(ColorSurface)
	ReviewCursorStyle = lipgloss.NewStyle().
		Background(ColorSurface)
	ReviewSearchMatchStyle = lipgloss.NewStyle().
		Background(ColorMuted)
	ReviewCurrentSearchMatchStyle = lipgloss.NewStyle().
		Background(ColorError)
	ReviewCommentedLineNumStyle = lipgloss.NewStyle().
		Foreground(ColorBackground).
		Background(ColorPrimary).
		Bold(true)
	ReviewSearchInputStyle = lipgloss.NewStyle().
		Foreground(ColorPrimary).
		Background(ColorBackground).
		Bold(true)
	ReviewModeNormalStyle = lipgloss.NewStyle().
		Foreground(ColorForeground).
		Background(ColorSurface).
		Bold(true).
		Padding(0, 1)
	ReviewModeVisualStyle = lipgloss.NewStyle().
		Foreground(ColorForeground).
		Background(ColorPrimary).
		Bold(true).
		Padding(0, 1)
	ReviewModeSearchStyle = lipgloss.NewStyle().
		Foreground(ColorForeground).
		Background(ColorError).
		Bold(true).
		Padding(0, 1)
	ReviewPosStyle = lipgloss.NewStyle().
		Foreground(ColorMuted).
		Background(ColorBackground).
		Padding(0, 1)
	ReviewHelpStyle = lipgloss.NewStyle().
		Foreground(ColorMuted).
		Background(ColorBackground).
		Padding(0, 1)
	ReviewStatusBarBgStyle = lipgloss.NewStyle().
		Background(ColorBackground)
	ReviewInlineCommentStyle = lipgloss.NewStyle().
		Foreground(ColorError).
		Background(ColorBackground).
		Padding(0, 1).
		Bold(true)

	ReviewTreeHeaderStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorForeground)
	ReviewTreeHeaderSelectedStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary)

	MessagesHelpStyle = lipgloss.NewStyle().
		Foreground(ColorMuted).
		PaddingLeft(1)
	MessagesPayloadSelectedStyle = lipgloss.NewStyle().
		Foreground(ColorForeground).
		Bold(true)

	CommandPaletteHelpSelectedStyle = lipgloss.NewStyle().
		Foreground(ColorPrimary).
		Faint(true)
	CommandPaletteMoreStyle = lipgloss.NewStyle().
		Foreground(ColorMuted).
		Italic(true)

	SelectFieldItemSelectedStyle = lipgloss.NewStyle().
		Foreground(ColorPrimary).
		Bold(true)

	ListFilterPromptStyle = lipgloss.NewStyle().
		PaddingLeft(1).
		Foreground(ColorPrimary).
		Bold(true)
	ListHelpContainerStyle = lipgloss.NewStyle().
		PaddingLeft(1)
	TabBrandingStyle = lipgloss.NewStyle().
		Background(ColorSurface).
		Foreground(ColorForeground).
		Padding(0, 1)
	PreviewHeaderNameStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary)
	PreviewTopicStyle = lipgloss.NewStyle().
		Foreground(ColorPrimary).
		Bold(true)
	PreviewSessionStyle = lipgloss.NewStyle().
		Foreground(ColorMuted).
		Italic(true)

	ColorPool = []color.Color{
		ColorPrimary,
		ColorSecondary,
		ColorSuccess,
		ColorWarning,
		ColorError,
		ColorMuted,
	}
}

// ColorForString returns a deterministic color for a given string.
// The same string always produces the same color.
func ColorForString(s string) color.Color {
	var hash uint32
	for _, c := range s {
		hash = hash*31 + uint32(c)
	}
	return ColorPool[hash%uint32(len(ColorPool))]
}

// nolint:gochecknoinits // bootstrap default theme before any style is accessed.
func init() {
	SetTheme(themes[DefaultTheme])
}
