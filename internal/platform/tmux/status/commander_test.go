package tmuxstatus

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExecCommanderRunsTmuxInHome(t *testing.T) {
	bin := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bin, "tmux"), []byte("#!/bin/sh\n/bin/pwd -P\n"), 0o755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	home, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	t.Setenv("HOME", home)
	t.Chdir(t.TempDir())

	out, err := execCommander{}.Output(context.Background(), "ignored")
	require.NoError(t, err)
	require.Equal(t, home, strings.TrimSpace(string(out)))
}
