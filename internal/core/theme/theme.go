// Package theme holds the built-in color palettes. It has no rendering
// dependency, so the packages that the CLI and Hive Desktop share can use it.
package theme

import (
	"fmt"
	"image/color"
	"sort"
	"strconv"
)

// Palette defines a minimal semantic theme palette.
type Palette struct {
	Primary    color.Color
	Secondary  color.Color
	Foreground color.Color
	Muted      color.Color
	Background color.Color
	Surface    color.Color
	// SurfaceLow is a subtle lift above Background (below Surface), used
	// for low-emphasis fills like selected-row highlights.
	SurfaceLow color.Color
	Success    color.Color
	Warning    color.Color
	Error      color.Color
}

// Default is the name of the default theme.
const Default = "tokyo-night"

// themes holds the built-in named palettes.
var themes = map[string]Palette{
	"tokyo-night": {
		Primary:    hex("#7aa2f7"),
		Secondary:  hex("#7dcfff"),
		Foreground: hex("#c0caf5"),
		Muted:      hex("#565f89"),
		Background: hex("#1a1b26"),
		Surface:    hex("#3b4261"),
		SurfaceLow: hex("#232433"),
		Success:    hex("#9ece6a"),
		Warning:    hex("#e0af68"),
		Error:      hex("#f7768e"),
	},
	"gruvbox": {
		Primary:    hex("#83a598"),
		Secondary:  hex("#8ec07c"),
		Foreground: hex("#ebdbb2"),
		Muted:      hex("#665c54"),
		Background: hex("#282828"),
		Surface:    hex("#3c3836"),
		SurfaceLow: hex("#32302f"), // dark0_soft
		Success:    hex("#b8bb26"),
		Warning:    hex("#fabd2f"),
		Error:      hex("#fb4934"),
	},
	"catppuccin": {
		Primary:    hex("#89b4fa"), // Blue
		Secondary:  hex("#94e2d5"), // Teal
		Foreground: hex("#cdd6f4"), // Text
		Muted:      hex("#6c7086"), // Overlay0
		Background: hex("#1e1e2e"), // Base
		Surface:    hex("#313244"), // Surface0
		SurfaceLow: hex("#272839"),
		Success:    hex("#a6e3a1"), // Green
		Warning:    hex("#f9e2af"), // Yellow
		Error:      hex("#f38ba8"), // Red
	},
	"kanagawa": {
		Primary:    hex("#7E9CD8"), // crystalBlue
		Secondary:  hex("#7FB4CA"), // springBlue
		Foreground: hex("#DCD7BA"), // fujiWhite
		Muted:      hex("#727169"), // fujiGray
		Background: hex("#1F1F28"), // sumiInk1
		Surface:    hex("#2A2A37"), // sumiInk3
		SurfaceLow: hex("#24242F"),
		Success:    hex("#76946A"), // autumnGreen
		Warning:    hex("#DCA561"), // autumnYellow
		Error:      hex("#C34043"), // autumnRed
	},
	"onedark": {
		Primary:    hex("#61afef"), // blue
		Secondary:  hex("#56b6c2"), // cyan
		Foreground: hex("#abb2bf"), // foreground
		Muted:      hex("#5c6370"), // comment grey
		Background: hex("#282c34"), // background
		Surface:    hex("#3e4452"), // gutter grey
		SurfaceLow: hex("#2c323c"), // cursor grey
		Success:    hex("#98c379"), // green
		Warning:    hex("#e5c07b"), // yellow
		Error:      hex("#e06c75"), // red
	},
}

// Names returns sorted names of all built-in themes.
func Names() []string {
	names := make([]string, 0, len(themes))
	for name := range themes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Get returns the palette for the given theme name.
func Get(name string) (Palette, bool) {
	p, ok := themes[name]
	return p, ok
}

// hex parses "#RRGGBB". It panics on a malformed value, because each caller
// passes a literal from the table in this file.
func hex(s string) color.Color {
	if len(s) != 7 || s[0] != '#' {
		panic(fmt.Sprintf("theme: malformed color %q", s))
	}
	v, err := strconv.ParseUint(s[1:], 16, 24)
	if err != nil {
		panic(fmt.Sprintf("theme: malformed color %q: %v", s, err))
	}
	return color.RGBA{
		R: uint8(v >> 16 & 0xff),
		G: uint8(v >> 8 & 0xff),
		B: uint8(v & 0xff),
		A: 0xff,
	}
}
