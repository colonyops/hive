// Package sshdial opens a connection to a remote hive daemon through
// `ssh <host> -- hive daemon dial-stdio`.
package sshdial

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strings"
	"sync"

	"github.com/colonyops/hive/internal/domain/server"
)

// Args are the ssh arguments for `<binary> daemon dial-stdio` on h.
// BatchMode and StrictHostKeyChecking keep ssh from prompting, since no one
// is at a terminal to answer.
func Args(h server.Host) []string {
	var args []string
	if h.ConfigFile != "" {
		args = append(args, "-F", h.ConfigFile)
	}
	args = append(args,
		"-T",
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=yes",
		"-o", "ConnectTimeout=10",
		"-o", "ServerAliveInterval=15",
		"-o", "ServerAliveCountMax=3",
		h.Name, "--", remoteBinary(h.Binary), "daemon", "dial-stdio",
	)
	return args
}

func remoteBinary(binary string) string {
	if binary == "" {
		return "hive"
	}
	if rest, ok := strings.CutPrefix(binary, "~/"); ok {
		return `"$HOME"/` + quote(rest)
	}
	return quote(binary)
}

func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// DialStdio starts ssh and returns its stdio as a connection. ssh's stderr
// goes to stderr when it is not nil. Closing the connection kills ssh and
// waits for it. The connection outlives ctx; ctx bounds only the start.
func DialStdio(ctx context.Context, h server.Host, stderr io.Writer) (net.Conn, error) {
	if err := h.Validate(); err != nil {
		return nil, err
	}
	// net.Pipe gives the HTTP/2 client deadlines and lets Close join both
	// copy loops.
	local, bridge := net.Pipe()
	cmd := exec.CommandContext(context.WithoutCancel(ctx), "ssh", Args(h)...) //nolint:gosec // Validate restricts every argument
	cmd.Stderr = stderr
	in, err := cmd.StdinPipe()
	if err != nil {
		_ = local.Close()
		_ = bridge.Close()
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		_ = in.Close()
		_ = local.Close()
		_ = bridge.Close()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		_ = in.Close()
		_ = out.Close()
		_ = local.Close()
		_ = bridge.Close()
		return nil, fmt.Errorf("start ssh: %w", err)
	}
	c := &conn{Conn: local, cmd: cmd}
	c.copies.Add(2)
	go func() {
		defer c.copies.Done()
		_, _ = io.Copy(in, bridge)
		_ = in.Close()
		_ = bridge.Close()
	}()
	go func() {
		defer c.copies.Done()
		if daemon, err := skipShellOutput(out, stderr); err == nil {
			_, _ = io.Copy(bridge, daemon)
		}
		_ = bridge.Close()
	}()
	if err := ctx.Err(); err != nil {
		_ = c.Close()
		return nil, err
	}
	return c, nil
}

// maxShellOutput limits what skipShellOutput discards, so a stream that never
// holds the daemon's first frame fails instead of filling memory.
const maxShellOutput = 64 << 10

// skipShellOutput returns r from the daemon's first byte. A shell startup file
// on the server can print text to stdout before hive runs, and HTTP/2 reads
// that text as a frame. An HTTP/2 server must send a SETTINGS frame first
// (RFC 9113, section 3.4), so the daemon starts at the first SETTINGS header.
func skipShellOutput(r io.Reader, stderr io.Writer) (io.Reader, error) {
	var seen []byte
	chunk := make([]byte, 4096)
	for {
		n, err := r.Read(chunk)
		seen = append(seen, chunk[:n]...)
		if i := settingsFrameIndex(seen); i >= 0 {
			return io.MultiReader(bytes.NewReader(seen[i:]), r), nil
		}
		if err != nil {
			return nil, err
		}
		if len(seen) > maxShellOutput {
			if stderr != nil {
				first, _, _ := bytes.Cut(seen, []byte("\n"))
				_, _ = fmt.Fprintf(stderr, "ssh printed %d bytes with no answer from the daemon; first line: %.200s\n", len(seen), first)
			}
			return nil, fmt.Errorf("no answer from the daemon in %d bytes of output", len(seen))
		}
	}
}

// settingsFrameIndex finds an HTTP/2 SETTINGS frame header: a 24-bit length that
// is a multiple of 6, type 0x4, no flags, and stream 0. The NUL bytes keep it
// from matching text.
func settingsFrameIndex(b []byte) int {
	const headerLen = 9
	for i := 0; i+headerLen <= len(b); i++ {
		h := b[i : i+headerLen]
		length := int(h[0])<<16 | int(h[1])<<8 | int(h[2])
		if h[0] == 0 && length%6 == 0 && bytes.Equal(h[3:], []byte{0x4, 0, 0, 0, 0, 0}) {
			return i
		}
	}
	return -1
}

type conn struct {
	net.Conn
	cmd    *exec.Cmd
	once   sync.Once
	copies sync.WaitGroup
}

func (c *conn) Close() error {
	var err error
	c.once.Do(func() {
		err = c.Conn.Close()
		_ = c.cmd.Process.Kill()
		_ = c.cmd.Wait()
		c.copies.Wait()
	})
	return err
}
