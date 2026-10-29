package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRef(t *testing.T) {
	tests := []struct {
		in   string
		want Ref
	}{
		{in: "abc123", want: Ref{Server: LocalServer, ID: "abc123"}},
		{in: "local:abc123", want: Ref{Server: LocalServer, ID: "abc123"}},
		{in: "box-1:abc123", want: Ref{Server: "box-1", ID: "abc123"}},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseRef(tt.in, LocalServer)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	for _, bad := range []string{"", "box:", ":abc123"} {
		_, err := ParseRef(bad, LocalServer)
		assert.Error(t, err, bad)
	}
}

func TestRefStringRoundTrips(t *testing.T) {
	for _, ref := range []Ref{LocalRef("abc123"), {Server: "box", ID: "abc123"}} {
		parsed, err := ParseRef(ref.String(), LocalServer)
		require.NoError(t, err)
		assert.Equal(t, ref, parsed)
	}
	assert.Equal(t, "abc123", LocalRef("abc123").String())
}

func TestParseRefPutsABareIDOnTheDefaultServer(t *testing.T) {
	ref, err := ParseRef("abc123", "box")
	require.NoError(t, err)
	assert.Equal(t, Ref{Server: "box", ID: "abc123"}, ref)
}

func TestValidateServerName(t *testing.T) {
	require.NoError(t, ValidateServerName("box-1.lan"))
	for _, bad := range []string{LocalServer, "a:b", "", "-x", "a b"} {
		assert.Error(t, ValidateServerName(bad), bad)
	}
}
