package validate

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSessionName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid name", "my-session", false},
		{"valid with spaces", "my session", false},
		{"empty string", "", true},
		{"only spaces", "   ", true},
		{"only tabs", "\t\t", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := SessionName(tt.input)
			assert.Equal(t, tt.wantErr, err != nil, "SessionName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
		})
	}
}

func TestSessionID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid alphanumeric", "abc123", false},
		{"valid letters only", "abcdef", false},
		{"valid numbers only", "123456", false},
		{"empty string", "", true},
		{"with spaces", "abc 123", true},
		{"with hyphen", "abc-123", true},
		{"with underscore", "abc_123", true},
		{"uppercase letters", "ABC123", true},
		{"mixed case", "AbC123", true},
		{"special chars", "abc!@#", true},
		{"unicode", "abc日本", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := SessionID(tt.input)
			assert.Equal(t, tt.wantErr, err != nil, "SessionID(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
		})
	}
}
