package httpapi

import (
	"os"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWireVersionsMatchTheFrontendConstants is the bijection between each
// socket's server-side version and the one its only client sends. Nothing else
// catches the drift: a version the server moved and the client did not is a 400
// at the handshake, which the frontend reports as a connection that dropped —
// the terminal opens over HTTP and then never streams. Every Go test dials with
// the Go constant, so the suites stay green while the app is broken.
//
// The two wires are versioned apart on purpose, and this is also what holds
// them apart: they carry different frames, so a change to one must not refuse
// the other's handshake.
func TestWireVersionsMatchTheFrontendConstants(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		path  string
		name  string
		want  string
		about string
	}{
		{
			path:  "../../../frontend/src/lib/terminalClient.ts",
			name:  "TERMINAL_WIRE_VERSION",
			want:  terminalWireVersion,
			about: "the tmux stream",
		},
		{
			path:  "../../../frontend/src/lib/popupTerminalClient.ts",
			name:  "POPUP_WIRE_VERSION",
			want:  ptyWireVersion,
			about: "the ptyterm stream",
		},
	} {
		source, err := os.ReadFile(tc.path)
		require.NoError(t, err, "the frontend client for %s must exist at %s", tc.about, tc.path)

		declaration := regexp.MustCompile(`export const ` + tc.name + ` = '([^']*)'`).FindSubmatch(source)
		require.NotNil(t, declaration, "could not find %s in %s", tc.name, tc.path)

		assert.Equal(t, tc.want, string(declaration[1]),
			"%s sends a version %s refuses; move both or the handshake fails", tc.name, tc.about)
	}
}
