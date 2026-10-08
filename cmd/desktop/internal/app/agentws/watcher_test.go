package agentws

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsWorkspaceFile(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{filepath.Join("/root", libraryFileName), true},
		{filepath.Join("/root", skillLibraryFileName), true},
		{filepath.Join("/root", "ws", manifestFileName), true},
		{filepath.Join("/root", "ws", "AGENTS.md"), false},
		{filepath.Join("/root", "ws", "CLAUDE.md"), false},
		{filepath.Join("/root", "ws", ".mcp.json"), false},
		{filepath.Join("/root", "ws", manifestFileName+".tmp"), false},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			assert.Equal(t, tc.want, isWorkspaceFile(tc.path))
		})
	}
}
