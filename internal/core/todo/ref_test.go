package todo

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRef(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		scheme  string
		value   string
		wantErr bool
		empty   bool
	}{
		{
			name:  "empty string",
			input: "",
			empty: true,
		},
		{
			name:   "session scheme",
			input:  "session://abc123",
			scheme: "session",
			value:  "abc123",
		},
		{
			name:   "https URL",
			input:  "https://github.com/org/repo",
			scheme: "https",
			value:  "github.com/org/repo",
		},
		{
			name:   "scheme lowered",
			input:  "HTTP://Example.Com",
			scheme: "http",
			value:  "Example.Com",
		},
		{
			name:    "bare string returns error",
			input:   "bare-string",
			wantErr: true,
		},
		{
			name:   "first separator only",
			input:  "review://path/with://colons",
			scheme: "review",
			value:  "path/with://colons",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, err := ParseRef(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.scheme, ref.Scheme())
			assert.Equal(t, tt.value, ref.Value())
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
		ref, err := ParseRef(input)
		require.NoError(t, err)
		assert.Equal(t, input, ref.String())

		ref2, err := ParseRef(ref.String())
		require.NoError(t, err)
		assert.Equal(t, ref, ref2)
	}
}

func TestRefTextMarshal(t *testing.T) {
	ref, err := ParseRef("review://docs/api.md")
	require.NoError(t, err)

	data, err := ref.MarshalText()
	require.NoError(t, err)
	assert.Equal(t, "review://docs/api.md", string(data))

	var ref2 Ref
	err = ref2.UnmarshalText(data)
	require.NoError(t, err)
	assert.Equal(t, ref, ref2)
}

func TestParseRefError(t *testing.T) {
	_, err := ParseRef("bare-string")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid URI")
}
