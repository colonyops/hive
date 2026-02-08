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
	"catppuccin": {
		Primary:    lipgloss.Color("#89b4fa"), // Blue
		Secondary:  lipgloss.Color("#94e2d5"), // Teal
		Foreground: lipgloss.Color("#cdd6f4"), // Text
		Muted:      lipgloss.Color("#6c7086"), // Overlay0
		Background: lipgloss.Color("#1e1e2e"), // Base
		Surface:    lipgloss.Color("#313244"), // Surface0
		Success:    lipgloss.Color("#a6e3a1"), // Green
		Warning:    lipgloss.Color("#f9e2af"), // Yellow
		Error:      lipgloss.Color("#f38ba8"), // Red
	},
	"kanagawa": {
		Primary:    lipgloss.Color("#7E9CD8"), // crystalBlue
		Secondary:  lipgloss.Color("#7FB4CA"), // springBlue
		Foreground: lipgloss.Color("#DCD7BA"), // fujiWhite
		Muted:      lipgloss.Color("#727169"), // fujiGray
		Background: lipgloss.Color("#1F1F28"), // sumiInk1
		Surface:    lipgloss.Color("#2A2A37"), // sumiInk3
		Success:    lipgloss.Color("#76946A"), // autumnGreen
		Warning:    lipgloss.Color("#DCA561"), // autumnYellow
		Error:      lipgloss.Color("#C34043"), // autumnRed
	},
	"onedark": {
		Primary:    lipgloss.Color("#61afef"), // blue
		Secondary:  lipgloss.Color("#56b6c2"), // cyan
		Foreground: lipgloss.Color("#abb2bf"), // foreground
		Muted:      lipgloss.Color("#5c6370"), // comment grey
		Background: lipgloss.Color("#282c34"), // background
		Surface:    lipgloss.Color("#3e4452"), // gutter grey
		Success:    lipgloss.Color("#98c379"), // green
		Warning:    lipgloss.Color("#e5c07b"), // yellow
		Error:      lipgloss.Color("#e06c75"), // red
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
