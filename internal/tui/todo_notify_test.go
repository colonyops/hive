package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTmuxPassthrough(t *testing.T) {
	t.Run("wraps sequence in DCS passthrough", func(t *testing.T) {
		seq := "\x1b]9;test\x07"
		result := tmuxPassthrough(seq)

		assert.Contains(t, result, "\x1bPtmux;")
		assert.Equal(t, "\x1b\\", result[len(result)-2:], "should end with ST")
	})

	t.Run("doubles ESC bytes", func(t *testing.T) {
		seq := "\x1b]9;Hello\x07\x1b]777;notify;Title;Body\x07"
		result := tmuxPassthrough(seq)

		// Each \x1b in original becomes \x1b\x1b inside the DCS body
		assert.Contains(t, result, "\x1b\x1b]9;Hello\x07")
		assert.Contains(t, result, "\x1b\x1b]777;notify;Title;Body\x07")
	})
}
