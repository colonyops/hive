package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSizeUnmarshalText(t *testing.T) {
	tests := []struct {
		in   string
		want Size
	}{
		{"5MiB", Size{Bytes: 5 << 20}},
		{"10GB", Size{Bytes: 10_000_000_000}},
		{"10GiB", Size{Bytes: 10 << 30}},
		{"1.5 KiB", Size{Bytes: 1536}},
		{"512", Size{Bytes: 512}},
		{" Unlimited ", Size{Unlimited: true}},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			var got Size
			require.NoError(t, got.UnmarshalText([]byte(tt.in)))
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSizeGBAndGiBDiffer(t *testing.T) {
	var gb, gib Size
	require.NoError(t, gb.UnmarshalText([]byte("1GB")))
	require.NoError(t, gib.UnmarshalText([]byte("1GiB")))
	assert.NotEqual(t, gb.Bytes, gib.Bytes)
}

func TestSizeUnmarshalTextRejects(t *testing.T) {
	for _, in := range []string{"0", "0GB", "-5MB", "", "GB", "5XB", "abc", "99999999999TiB"} {
		t.Run(in, func(t *testing.T) {
			var got Size
			require.Error(t, got.UnmarshalText([]byte(in)))
			assert.False(t, got.IsSet())
		})
	}
}
