// Package tui implements the Bubble Tea TUI for hive.
package tui

import (
	"image/color"

	"github.com/hay-kot/hive/internal/core/styles"
)

// Icons and symbols.
const (
	iconDot = "•" // Unicode bullet separator
)

// ColorForString returns a deterministic color for a given string.
// The same string always produces the same color.
func ColorForString(s string) color.Color {
	return styles.ColorForString(s)
}
