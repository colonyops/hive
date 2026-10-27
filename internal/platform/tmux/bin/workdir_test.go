package tmuxbin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWorkDir(t *testing.T) {
	home := t.TempDir()
	file := filepath.Join(home, "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))

	tests := []struct {
		name string
		home string
		want string
	}{
		{name: "home", home: home, want: home},
		{name: "unset", home: "", want: "/"},
		{name: "missing", home: filepath.Join(home, "gone"), want: "/"},
		{name: "not a directory", home: file, want: "/"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, workDir(tt.home))
		})
	}
}
