package todo

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRef(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		scheme string
		value  string
		valid  bool
		empty  bool
	}{
		{
			name:  "empty string",
			input: "",
			valid: true,
			empty: true,
		},
		{
			name:   "session scheme",
			input:  "session://abc123",
			scheme: "session",
			value:  "abc123",
			valid:  true,
		},
		{
			name:   "https URL",
			input:  "https://github.com/org/repo",
			scheme: "https",
			value:  "github.com/org/repo",
			valid:  true,
		},
		{
			name:   "scheme lowered",
			input:  "HTTP://Example.Com",
			scheme: "http",
			value:  "Example.Com",
			valid:  true,
		},
		{
			name:  "bare string invalid",
			input: "bare-string",
			value: "bare-string",
			valid: false,
		},
		{
			name:   "first separator only",
			input:  "review://path/with://colons",
			scheme: "review",
			value:  "path/with://colons",
			valid:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref := ParseRef(tt.input)
			assert.Equal(t, tt.scheme, ref.Scheme)
			assert.Equal(t, tt.value, ref.Value)
			assert.Equal(t, tt.valid, ref.Valid())
			assert.Equal(t, tt.empty, ref.IsEmpty())
		})
	}
}

func TestRefRoundTrip(t *testing.T) {
	inputs := []string{
		"session://abc123",
		"https://github.com/org/repo",
		"review://docs/api.md",
	}

	for _, input := range inputs {
		ref := ParseRef(input)
		assert.Equal(t, input, ref.String())
		assert.Equal(t, ref, ParseRef(ref.String()))
	}
}

func TestRefTextMarshal(t *testing.T) {
	ref := ParseRef("review://docs/api.md")

	data, err := ref.MarshalText()
	require.NoError(t, err)
	assert.Equal(t, "review://docs/api.md", string(data))

	var ref2 Ref
	err = ref2.UnmarshalText(data)
	require.NoError(t, err)
	assert.Equal(t, ref, ref2)
}
