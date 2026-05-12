package timer

import (
	"strings"
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		want    time.Duration
	}{
		{name: "below min 500ms", input: "500ms", wantErr: true},
		{name: "zero", input: "0", wantErr: true},
		{name: "negative", input: "-5s", wantErr: true},
		{name: "min boundary 1s", input: "1s", want: 1 * time.Second},
		{name: "one minute", input: "1m", want: 1 * time.Minute},
		{name: "max boundary 4h", input: "4h", want: 4 * time.Hour},
		{name: "over max 4h1s", input: "4h1s", wantErr: true},
		{name: "malformed abc", input: "abc", wantErr: true},
		{name: "empty string", input: "", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseDuration(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseDuration(%q) = %v, want error", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseDuration(%q) returned unexpected error: %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("ParseDuration(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestValidatePrompt(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "empty", input: "", wantErr: true},
		{name: "single byte", input: "a"},
		{name: "exact max ascii", input: strings.Repeat("a", MaxPromptSize)},
		{name: "over max ascii", input: strings.Repeat("a", MaxPromptSize+1), wantErr: true},
		{name: "exact max multi-byte utf8", input: strings.Repeat("é", MaxPromptSize/2)},
		{name: "over max multi-byte utf8", input: strings.Repeat("é", MaxPromptSize/2) + "a", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidatePrompt(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ValidatePrompt(len=%d) = nil, want error", len(tc.input))
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidatePrompt(len=%d) returned unexpected error: %v", len(tc.input), err)
			}
		})
	}
}
