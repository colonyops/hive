package sshdial

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/domain/server"
)

func TestArgs(t *testing.T) {
	want := []string{
		"-F", "/state/ssh_config",
		"-T",
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=yes",
		"-o", "ConnectTimeout=10",
		"-o", "ServerAliveInterval=15",
		"-o", "ServerAliveCountMax=3",
		"fixture", "--", `"$HOME"/'go/bin/hive'`, "daemon", "dial-stdio",
	}
	assert.Equal(t, want, Args(server.Host{Name: "fixture", ConfigFile: "/state/ssh_config", Binary: "~/go/bin/hive"}))

	args := Args(server.Host{Name: "box"})
	assert.Equal(t, []string{"box", "--", "hive", "daemon", "dial-stdio"}, args[len(args)-5:])
	args = Args(server.Host{Name: "box", Binary: "/usr/local/bin/hive"})
	assert.Equal(t, "'/usr/local/bin/hive'", args[len(args)-3])
}

// fakeSSH puts an ssh on PATH that records its pid and then runs body.
func fakeSSH(t *testing.T, body string) (pidFile string) {
	t.Helper()
	dir := t.TempDir()
	pidFile = filepath.Join(dir, "pid")
	script := "#!/bin/sh\necho $$ > " + pidFile + "\n" + body + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ssh"), []byte(script), 0o755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return pidFile
}

func TestDialStdioRoundTripsAndKillsSSHOnClose(t *testing.T) {
	pidFile := fakeSSH(t, "exec cat")
	c, err := DialStdio(t.Context(), server.Host{Name: "fixture"}, nil)
	require.NoError(t, err)

	_, err = c.Write([]byte("ping"))
	require.NoError(t, err)
	buf := make([]byte, 4)
	_, err = io.ReadFull(c, buf)
	require.NoError(t, err)
	assert.Equal(t, "ping", string(buf))

	sshProc := c.(*conn).cmd
	require.NoError(t, c.Close())
	assert.NotNil(t, sshProc.ProcessState, "Close waits for ssh to exit")
	assert.FileExists(t, pidFile)

	done := make(chan struct{})
	go func() { _ = c.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("a second Close blocked")
	}
}

func TestDialStdioReportsEOFWhenSSHExits(t *testing.T) {
	fakeSSH(t, "exit 255")
	c, err := DialStdio(t.Context(), server.Host{Name: "fixture"}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	read := make(chan error, 1)
	go func() {
		_, err := c.Read(make([]byte, 1))
		read <- err
	}()
	select {
	case err := <-read:
		require.Error(t, err, "a client must not wait on a dead ssh")
	case <-time.After(5 * time.Second):
		t.Fatal("Read blocked after ssh exited")
	}
}
