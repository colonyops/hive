package tui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTailLines(t *testing.T) {
	tests := []struct {
		name  string
		input string
		n     int
		want  string
	}{
		{"zero returns empty", "a\nb\nc", 0, ""},
		{"all lines fit", "a\nb", 5, "a\nb"},
		{"returns last n", "a\nb\nc\nd", 2, "c\nd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tailLines(tt.input, tt.n))
		})
	}
}

func TestTruncateLines(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxWidth int
		want     string
	}{
		{"zero width returns input", "hello", 0, "hello"},
		{"short lines unchanged", "hi\nok", 10, "hi\nok"},
		{"long line truncated", "abcdefghij", 5, "abcde"},
		{"multiline truncates only long", "hi\nabcdefghij\nok", 5, "hi\nabcde\nok"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, truncateLines(tt.input, tt.maxWidth))
		})
	}
}

func TestEnsureExactWidth(t *testing.T) {
	tests := []struct {
		name  string
		input string
		width int
	}{
		{"zero width returns input", "hello", 0},
		{"short line padded", "hi", 6},
		{"long line truncated", "abcdefghij", 5},
		{"exact width unchanged", "abcde", 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ensureExactWidth(tt.input, tt.width)
			if tt.width <= 0 {
				assert.Equal(t, tt.input, got)
				return
			}
			for _, line := range strings.Split(got, "\n") {
				assert.Len(t, line, tt.width)
			}
		})
	}
}

func TestEnsureExactHeight(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		n         int
		wantLines int
		wantExact string
	}{
		{"zero returns empty", "a\nb", 0, 0, ""},
		{"truncates preserving first", "first\nsecond\nthird", 2, 2, "first\nsecond"},
		{"pads short content", "a", 3, 3, "a\n\n"},
		{"exact height unchanged", "a\nb\nc", 3, 3, "a\nb\nc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ensureExactHeight(tt.input, tt.n)
			assert.Equal(t, tt.wantExact, got)
		})
	}
}

func TestItoa(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{0, "0"},
		{42, "42"},
		{-7, "-7"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, itoa(tt.input))
		})
	}
}

func TestIsFilterAction(t *testing.T) {
	tests := []struct {
		action string
		want   bool
	}{
		{"filter-all", true},
		{"filter-active", true},
		{"filter-approval", true},
		{"filter-ready", true},
		{"delete", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			assert.Equal(t, tt.want, isFilterAction(tt.action))
		})
	}
}
