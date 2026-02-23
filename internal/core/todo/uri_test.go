package todo

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseURI(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  URI
	}{
		{name: "empty string", input: "", want: URI{}},
		{name: "session URI", input: "session://abc123", want: URI{Scheme: "session", Value: "abc123"}},
		{name: "https URL", input: "https://github.com/org/repo", want: URI{Scheme: "https", Value: "github.com/org/repo"}},
		{name: "http URL", input: "http://example.com/path", want: URI{Scheme: "http", Value: "example.com/path"}},
		{name: "uppercase scheme lowered", input: "HTTP://Example.Com", want: URI{Scheme: "http", Value: "Example.Com"}},
		{name: "bare string", input: "bare-string", want: URI{Value: "bare-string"}},
		{name: "code-review URI", input: "code-review://pr-42", want: URI{Scheme: "code-review", Value: "pr-42"}},
		{name: "value with colons", input: "review://path/with://colons", want: URI{Scheme: "review", Value: "path/with://colons"}},
		{name: "obsidian URI", input: "obsidian://open?vault=notes&file=todo", want: URI{Scheme: "obsidian", Value: "open?vault=notes&file=todo"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseURI(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestURI_String(t *testing.T) {
	assert.Equal(t, "session://abc123", URI{Scheme: "session", Value: "abc123"}.String())
	assert.Empty(t, URI{}.String())
	assert.Equal(t, "bare-string", URI{Value: "bare-string"}.String())
}

func TestURI_IsZero(t *testing.T) {
	assert.True(t, URI{}.IsZero())
	assert.False(t, URI{Scheme: "session", Value: "abc"}.IsZero())
	assert.False(t, URI{Value: "bare"}.IsZero())
}

func TestURI_Valid(t *testing.T) {
	assert.True(t, URI{}.Valid(), "empty URI is valid")
	assert.True(t, URI{Scheme: "session", Value: "abc"}.Valid(), "URI with scheme is valid")
	assert.False(t, URI{Value: "bare-string"}.Valid(), "bare string is invalid")
}

func TestParseURI_Roundtrip(t *testing.T) {
	inputs := []string{
		"session://abc123",
		"https://github.com/org/repo/pull/42",
		"code-review://pr-42",
	}
	for _, input := range inputs {
		got := ParseURI(input)
		assert.Equal(t, input, got.String())
		roundtrip := ParseURI(got.String())
		assert.Equal(t, got, roundtrip)
	}
}

func TestURI_MarshalText(t *testing.T) {
	uri := URI{Scheme: "session", Value: "abc123"}
	data, err := uri.MarshalText()
	require.NoError(t, err)
	assert.Equal(t, "session://abc123", string(data))

	var decoded URI
	err = decoded.UnmarshalText(data)
	require.NoError(t, err)
	assert.Equal(t, uri, decoded)
}
