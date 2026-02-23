package action

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseType_CaseInsensitive(t *testing.T) {
	tests := []struct {
		input string
		want  Type
	}{
		{"Recycle", TypeRecycle},
		{"recycle", TypeRecycle},
		{"RECYCLE", TypeRecycle},
		{"Delete", TypeDelete},
		{"delete", TypeDelete},
		{"Todos", TypeTodos},
		{"todos", TypeTodos},
		{"FilterAll", TypeFilterAll},
		{"filterall", TypeFilterAll},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseType(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseType_Invalid(t *testing.T) {
	_, err := ParseType("nonexistent")
	require.ErrorIs(t, err, ErrInvalidType)
}

func TestType_UnmarshalText(t *testing.T) {
	var typ Type
	err := typ.UnmarshalText([]byte("recycle"))
	require.NoError(t, err)
	assert.Equal(t, TypeRecycle, typ)
}

func TestType_UnmarshalText_PascalCase(t *testing.T) {
	var typ Type
	err := typ.UnmarshalText([]byte("Recycle"))
	require.NoError(t, err)
	assert.Equal(t, TypeRecycle, typ)
}

func TestTypeNames_ContainsTodos(t *testing.T) {
	names := TypeNames()
	assert.Contains(t, names, "Todos")
}
