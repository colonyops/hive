package jsoncolor

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestColorize_ValidJSON(t *testing.T) {
	input := []byte(`{"name":"test","count":42,"active":true,"tags":["a","b"],"meta":null}`)
	result := Colorize(input)

	// Should contain pretty-printed structure
	assert.Contains(t, result, "name")
	assert.Contains(t, result, "test")
	assert.Contains(t, result, "42")
	assert.Contains(t, result, "true")
	assert.Contains(t, result, "null")
	// Should be multi-line (indented)
	assert.Contains(t, result, "\n", "expected multi-line output")
}

func TestColorize_InvalidJSON(t *testing.T) {
	input := []byte(`not json at all`)
	result := Colorize(input)
	assert.Equal(t, "not json at all", result)
}

func TestColorize_EmptyObject(t *testing.T) {
	result := Colorize([]byte(`{}`))
	assert.Contains(t, result, "{")
	assert.Contains(t, result, "}")
}

func TestColorize_NestedJSON(t *testing.T) {
	input := []byte(`{"outer":{"inner":"value"}}`)
	result := Colorize(input)
	assert.Contains(t, result, "outer")
	assert.Contains(t, result, "inner")
	assert.Contains(t, result, "value")
}

func TestColorize_EscapedStrings(t *testing.T) {
	input := []byte(`{"msg":"hello \"world\""}`)
	result := Colorize(input)
	assert.Contains(t, result, `hello \"world\"`)
}

func TestColorize_Numbers(t *testing.T) {
	input := []byte(`{"int":42,"float":3.14,"neg":-1,"exp":1e10}`)
	result := Colorize(input)
	assert.Contains(t, result, "42")
	assert.Contains(t, result, "3.14")
	assert.Contains(t, result, "-1")
	assert.Contains(t, result, "1e10")
}

func TestColorize_BooleanAndNull(t *testing.T) {
	input := []byte(`{"t":true,"f":false,"n":null}`)
	result := Colorize(input)
	assert.Contains(t, result, "true")
	assert.Contains(t, result, "false")
	assert.Contains(t, result, "null")
}

func TestFindStringEnd(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		pos      int
		expected int
	}{
		{"simple", `"hello"`, 0, 6},
		{"escaped quote", `"he\"llo"`, 0, 8},
		{"escaped backslash", `"he\\"`, 0, 5},
		{"empty string", `""`, 0, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findStringEnd(tt.input, tt.pos)
			assert.Equal(t, tt.expected, got)
		})
	}
}
