package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsDecorativeLine(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"empty string", "", true},
		{"only spaces", "   ", true},
		{"horizontal rule dashes", "────────", true},
		{"horizontal rule hyphens", "-------", true},
		{"horizontal rule equals", "=======", true},
		{"mixed rule chars", "─━-=", true},
		{"text content", "hello world", false},
		{"rule with text", "── hello ──", false},
		{"ansi codes around rule", "\x1b[0m────\x1b[0m", true},
		{"ansi codes around text", "\x1b[31mhello\x1b[0m", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isDecorativeLine(tt.input))
		})
	}
}

func TestStripLeadingDecorative(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no decorative lines", "hello\nworld", "hello\nworld"},
		{"leading empty and rule", "\n────\nhello", "hello"},
		{"all decorative", "────\n───", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, stripLeadingDecorative(tt.input))
		})
	}
}

func TestStripTrailingDecorative(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no trailing decorative", "hello\nworld", "hello\nworld"},
		{"trailing empty and rule", "hello\n────\n", "hello"},
		{"all decorative", "────\n───", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, stripTrailingDecorative(tt.input))
		})
	}
}
