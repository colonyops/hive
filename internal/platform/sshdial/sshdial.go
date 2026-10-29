// Package sshdial opens a connection to a remote hive daemon through
// `ssh <host> -- hive daemon dial-stdio`.
package sshdial

import (
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
		_, _ = io.Copy(bridge, out)
		_ = bridge.Close()
	}()
	if err := ctx.Err(); err != nil {
		_ = c.Close()
		return nil, err
	}
	return c, nil
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
