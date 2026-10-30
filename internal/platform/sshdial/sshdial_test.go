package sshdial

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
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

// emptySettings is the HTTP/2 frame header of an empty SETTINGS frame.
var emptySettings = []byte{0, 0, 0, 0x4, 0, 0, 0, 0, 0}

func TestDialStdioDropsWhatTheShellPrintsBeforeTheDaemon(t *testing.T) {
	fakeSSH(t, "echo 'Welcome to box'\nexec cat")
	c, err := DialStdio(t.Context(), server.Host{Name: "fixture"}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	_, err = c.Write(emptySettings)
	require.NoError(t, err)
	buf := make([]byte, len(emptySettings))
	_, err = io.ReadFull(c, buf)
	require.NoError(t, err)
	assert.Equal(t, emptySettings, buf)
}

func TestSkipShellOutput(t *testing.T) {
	settings := []byte{0, 0, 18, 0x4, 0, 0, 0, 0, 0, 1, 2, 3}
	tests := []struct {
		name  string
		input []byte
		want  []byte
	}{
		{name: "no shell output", input: settings, want: settings},
		{name: "text first", input: append([]byte("Welcome\nyou have mail\n"), settings...), want: settings},
		{name: "no line end before the frame", input: append([]byte("no newline"), settings...), want: settings},
		{name: "a settings ack is not the first frame", input: append([]byte{0, 0, 0, 0x4, 1, 0, 0, 0, 0}, settings...), want: settings},
		{name: "a length that is not a multiple of 6", input: append([]byte{0, 0, 7, 0x4, 0, 0, 0, 0, 0}, settings...), want: settings},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := skipShellOutput(bytes.NewReader(tt.input), nil)
			require.NoError(t, err)
			got, err := io.ReadAll(r)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSkipShellOutputStopsOnEndlessText(t *testing.T) {
	var stderr bytes.Buffer
	endless := strings.NewReader("first line\n" + strings.Repeat("x", 2*maxShellOutput))

	_, err := skipShellOutput(endless, &stderr)

	require.Error(t, err)
	assert.Contains(t, stderr.String(), "first line: first line")
}

func TestSkipShellOutputReportsAClosedStream(t *testing.T) {
	_, err := skipShellOutput(strings.NewReader("hive: command not found\n"), nil)

	assert.ErrorIs(t, err, io.EOF)
}

func TestDialStdioRoundTripsAndKillsSSHOnClose(t *testing.T) {
	pidFile := fakeSSH(t, "exec cat")
	c, err := DialStdio(t.Context(), server.Host{Name: "fixture"}, nil)
	require.NoError(t, err)

	sent := append(append([]byte{}, emptySettings...), "ping"...)
	_, err = c.Write(sent)
	require.NoError(t, err)
	buf := make([]byte, len(sent))
	_, err = io.ReadFull(c, buf)
	require.NoError(t, err)
	assert.Equal(t, sent, buf)

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
