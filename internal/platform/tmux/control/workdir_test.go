package tmuxcc

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTmuxExecsRunInHome(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "tmux")
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\n/bin/pwd -P\n"), 0o755))
	home, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	t.Setenv("HOME", home)
	t.Setenv("TMUX", "")
	t.Chdir(t.TempDir())
	ctx := t.Context()

	lines, err := runTmux(ctx, bin, nil, "ignored")
	require.NoError(t, err)
	require.Equal(t, []string{home}, lines)

	version, err := tmuxVersion(ctx, bin)
	require.NoError(t, err)
	require.Equal(t, home, strings.TrimSpace(version))

	p := newExecProcess(Options{Slug: "s", Binary: bin})
	_, stdout, err := p.Start(ctx)
	require.NoError(t, err)
	line, err := bufio.NewReader(stdout).ReadString('\n')
	require.NoError(t, err)
	require.NoError(t, p.Wait())
	require.Equal(t, home, strings.TrimSpace(line))
}
