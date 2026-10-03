package terminal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStripANSI(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "no ansi",
			content: "plain text",
			want:    "plain text",
		},
		{
			name:    "color code",
			content: "\x1b[32mgreen\x1b[0m",
			want:    "green",
		},
		{
			name:    "multiple codes",
			content: "\x1b[1m\x1b[31mbold red\x1b[0m normal",
			want:    "bold red normal",
		},
		{
			name:    "cursor movement",
			content: "\x1b[2Amove up",
			want:    "move up",
		},
		{
			name:    "osc sequence with bell",
			content: "before\x1b]0;title\x07after",
			want:    "beforeafter",
		},
		{
			name:    "empty string",
			content: "",
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripANSI(tt.content)
			assert.Equal(t, tt.want, got, "StripANSI() = %v, want %v", got, tt.want)
		})
	}
}
