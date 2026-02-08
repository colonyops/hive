// Package styles provides shared lipgloss v2 styles for CLI and TUI components.
package styles

import (
	"image/color"
	"sort"

	lipgloss "charm.land/lipgloss/v2"
	glamouransi "github.com/charmbracelet/glamour/ansi"
	glamourstyles "github.com/charmbracelet/glamour/styles"
	"github.com/lucasb-eyer/go-colorful"
)

// Palette defines a minimal semantic theme palette.
type Palette struct {
	Primary    color.Color
	Secondary  color.Color
	Foreground color.Color
	Muted      color.Color
	Background color.Color
	Surface    color.Color
	Success    color.Color
	Warning    color.Color
	Error      color.Color
}

// DefaultTheme is the name of the default theme.
const DefaultTheme = "tokyo-night"

// themes holds the built-in named palettes.
var themes = map[string]Palette{
	"tokyo-night": {
		Primary:    lipgloss.Color("#7aa2f7"),
		Secondary:  lipgloss.Color("#7dcfff"),
		Foreground: lipgloss.Color("#c0caf5"),
		Muted:      lipgloss.Color("#565f89"),
		Background: lipgloss.Color("#1a1b26"),
		Surface:    lipgloss.Color("#3b4261"),
		Success:    lipgloss.Color("#9ece6a"),
		Warning:    lipgloss.Color("#e0af68"),
		Error:      lipgloss.Color("#f7768e"),
	},
	"gruvbox": {
		Primary:    lipgloss.Color("#83a598"),
		Secondary:  lipgloss.Color("#8ec07c"),
		Foreground: lipgloss.Color("#ebdbb2"),
		Muted:      lipgloss.Color("#665c54"),
		Background: lipgloss.Color("#282828"),
		Surface:    lipgloss.Color("#3c3836"),
		Success:    lipgloss.Color("#b8bb26"),
		Warning:    lipgloss.Color("#fabd2f"),
		Error:      lipgloss.Color("#fb4934"),
	},
}

// ThemeNames returns sorted names of all built-in themes.
func ThemeNames() []string {
	names := make([]string, 0, len(themes))
	for name := range themes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// GetPalette returns the palette for the given theme name.
func GetPalette(name string) (Palette, bool) {
	p, ok := themes[name]
	return p, ok
}

// CurrentPalette holds the active theme palette.
var CurrentPalette Palette

// Exported color aliases for convenience and compatibility.
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

	// Legacy aliases (map to semantic colors).
	ColorGreen     color.Color
	ColorYellow    color.Color
	ColorBlue      color.Color
	ColorCyan      color.Color
	ColorGray      color.Color
	ColorWhite     color.Color
	ColorPurple    color.Color
	ColorOrange    color.Color
	ColorTeal      color.Color
	ColorPink      color.Color
	ColorRedAlt    color.Color
	ColorLightGray color.Color
	ColorTextMuted color.Color

	ColorBgDark      color.Color
	ColorBgDarker    color.Color
	ColorBgVeryDark  color.Color
	ColorBgSelection color.Color
	ColorBgCursor    color.Color
)

// Style exports.
var (
	// CLI styles.
	CommandHeaderStyle lipgloss.Style
	CommandStyle       lipgloss.Style
	DividerStyle       lipgloss.Style

	// TUI shared styles.
	SelectedBorderStyle      lipgloss.Style
	ModalStyle               lipgloss.Style
	ModalTitleStyle          lipgloss.Style
	ModalHelpStyle           lipgloss.Style
	ModalButtonStyle         lipgloss.Style
	ModalButtonSelectedStyle lipgloss.Style

	ViewSelectedStyle lipgloss.Style
	ViewNormalStyle   lipgloss.Style

	GitAdditionsStyle lipgloss.Style
	GitDeletionsStyle lipgloss.Style
	GitCleanStyle     lipgloss.Style
	GitDirtyStyle     lipgloss.Style
	GitLoadingStyle   lipgloss.Style

	FormTitleStyle        lipgloss.Style
	FormTitleBlurredStyle lipgloss.Style
	FormFieldStyle        lipgloss.Style
	FormFieldFocusedStyle lipgloss.Style
	FormErrorStyle        lipgloss.Style
	FormHelpStyle         lipgloss.Style

	// Message preview modal styles.
	PreviewTopicStyle   lipgloss.Style
	PreviewSenderStyle  lipgloss.Style
	PreviewTimeStyle    lipgloss.Style
	PreviewSessionStyle lipgloss.Style
	PreviewDividerStyle lipgloss.Style
	PreviewScrollStyle  lipgloss.Style
	PreviewCopiedStyle  lipgloss.Style
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

	ColorGreen = p.Success
	ColorYellow = p.Warning
	ColorBlue = p.Primary
	ColorCyan = p.Secondary
	ColorGray = p.Muted
	ColorWhite = p.Foreground
	ColorPurple = lipgloss.Color("#bb9af7") // Tokyo Night purple
	ColorOrange = p.Warning
	ColorTeal = p.Secondary
	ColorPink = p.Error
	ColorRedAlt = p.Error
	ColorLightGray = p.Muted
	ColorTextMuted = p.Muted

	ColorBgDark = p.Background
	ColorBgDarker = p.Background
	ColorBgVeryDark = p.Background
	ColorBgSelection = p.Surface
	ColorBgCursor = p.Surface

	CommandHeaderStyle = lipgloss.NewStyle().
		Foreground(ColorPrimary).
		Bold(true)
	CommandStyle = lipgloss.NewStyle().
		Foreground(ColorForeground)
	DividerStyle = lipgloss.NewStyle().
		Foreground(ColorMuted)

	SelectedBorderStyle = lipgloss.NewStyle().
		Foreground(ColorPrimary)

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

	ViewSelectedStyle = lipgloss.NewStyle().
		Foreground(ColorPrimary).
		Bold(true)
	ViewNormalStyle = lipgloss.NewStyle().
		Foreground(ColorMuted)

	GitAdditionsStyle = lipgloss.NewStyle().Foreground(ColorSuccess)
	GitDeletionsStyle = lipgloss.NewStyle().Foreground(ColorError)
	GitCleanStyle = lipgloss.NewStyle().Foreground(ColorMuted)
	GitDirtyStyle = lipgloss.NewStyle().Foreground(ColorWarning)
	GitLoadingStyle = lipgloss.NewStyle().Foreground(ColorMuted)

	FormTitleStyle = lipgloss.NewStyle().
		Foreground(ColorPrimary).
		Bold(true)
	FormTitleBlurredStyle = lipgloss.NewStyle().
		Foreground(ColorMuted)
	FormFieldStyle = lipgloss.NewStyle().
		Border(lipgloss.ThickBorder(), false, false, false, true).
		BorderForeground(ColorMuted).
		PaddingLeft(1)
	FormFieldFocusedStyle = lipgloss.NewStyle().
		Border(lipgloss.ThickBorder(), false, false, false, true).
		BorderForeground(ColorPrimary).
		PaddingLeft(1)
	FormErrorStyle = lipgloss.NewStyle().
		Foreground(ColorError)
	FormHelpStyle = lipgloss.NewStyle().
		Foreground(ColorMuted)

	PreviewTopicStyle = lipgloss.NewStyle().
		Foreground(ColorPrimary).
		Bold(true)
	PreviewSenderStyle = lipgloss.NewStyle().
		Foreground(ColorSuccess)
	PreviewTimeStyle = lipgloss.NewStyle().
		Foreground(ColorMuted)
	PreviewSessionStyle = lipgloss.NewStyle().
		Foreground(ColorMuted).
		Italic(true)
	PreviewDividerStyle = lipgloss.NewStyle().
		Foreground(ColorSurface)
	PreviewScrollStyle = lipgloss.NewStyle().
		Foreground(ColorMuted)
	PreviewCopiedStyle = lipgloss.NewStyle().
		Foreground(ColorSuccess)

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

func colorHexPtr(c color.Color) *string {
	if c == nil {
		return nil
	}
	cc, ok := colorful.MakeColor(c)
	if !ok {
		return nil
	}
	hex := cc.Hex()
	return &hex
}

// GlamourStyle returns a Glamour style config derived from the active theme.
func GlamourStyle() glamouransi.StyleConfig {
	cfg := glamourstyles.DarkStyleConfig

	fg := colorHexPtr(ColorForeground)
	primary := colorHexPtr(ColorPrimary)
	secondary := colorHexPtr(ColorSecondary)
	muted := colorHexPtr(ColorMuted)
	surface := colorHexPtr(ColorSurface)

	cfg.Document.Color = fg

	cfg.Paragraph.Color = fg

	cfg.Heading.Color = primary
	cfg.H1.Color = fg
	cfg.H1.BackgroundColor = surface
	cfg.H2.Color = primary
	cfg.H3.Color = primary
	cfg.H4.Color = primary
	cfg.H5.Color = primary
	cfg.H6.Color = primary

	cfg.BlockQuote.Color = muted
	cfg.HorizontalRule.Color = muted

	cfg.Link.Color = secondary
	cfg.LinkText.Color = secondary

	cfg.Code.Color = secondary
	cfg.CodeBlock.Color = muted

	cfg.Table.Color = fg

	return cfg
}
