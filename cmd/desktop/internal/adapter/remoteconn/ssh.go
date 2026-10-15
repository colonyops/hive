package remoteconn

import (
	"context"
	"fmt"
	"io"
	"net"
	"os/exec"
	"regexp"
	"strings"
	"sync"
)

type Profile struct {
	Host           string `json:"host"`
	ConnectionFile string `json:"connectionFile"`
}

var hostPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.@:-]*$`)

func (p Profile) Validate() error {
	if !hostPattern.MatchString(p.Host) {
		return fmt.Errorf("enter an SSH host or config alias, optionally with user@")
	}
	if p.ConnectionFile == "" || strings.ContainsAny(p.ConnectionFile, "\x00\r\n") {
		return fmt.Errorf("enter the remote connection file path")
	}
	return nil
}

func sshArgs(p Profile) []string {
	return []string{"-T", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes", "-o", "ConnectTimeout=10", "-o", "ServerAliveInterval=15", "-o", "ServerAliveCountMax=3"}
}
func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }

func bootstrap(ctx context.Context, p Profile) ([]byte, error) {
	path := quote(p.ConnectionFile)
	if relative, ok := strings.CutPrefix(p.ConnectionFile, "~/"); ok {
		path = `"$HOME"/` + quote(relative)
	}
	args := append(sshArgs(p), p.Host, "cat -- "+path)
	cmd := exec.CommandContext(ctx, "ssh", args...)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		return nil, fmt.Errorf("start SSH: %w", err)
	}
	data, readErr := io.ReadAll(io.LimitReader(out, 64*1024+1))
	if readErr != nil || len(data) > 64*1024 {
		_ = cmd.Process.Kill()
	}
	waitErr := cmd.Wait()
	if readErr != nil {
		return nil, readErr
	}
	if len(data) > 64*1024 {
		return nil, fmt.Errorf("remote connection file exceeds 64 KiB")
	}
	if waitErr != nil {
		return nil, fmt.Errorf("SSH could not read the connection file; check the SSH host, known_hosts, agent, and file path: %w", waitErr)
	}
	return data, nil
}

type sshConn struct {
	net.Conn
	cmd    *exec.Cmd
	once   sync.Once
	copies sync.WaitGroup
}

func (c *sshConn) Close() error {
	var err error
	c.once.Do(func() {
		err = c.Conn.Close()
		_ = c.cmd.Process.Kill()
		_ = c.cmd.Wait()
		c.copies.Wait()
	})
	return err
}

func dialSSH(ctx context.Context, p Profile, address string) (net.Conn, error) {
	// net.Pipe supplies deadlines to TLS and joins both copy loops on Close.
	local, bridge := net.Pipe()
	cmd := exec.CommandContext(context.WithoutCancel(ctx), "ssh", append(sshArgs(p), "-W", address, p.Host)...)
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
	if err = cmd.Start(); err != nil {
		_ = in.Close()
		_ = out.Close()
		_ = local.Close()
		_ = bridge.Close()
		return nil, err
	}
	c := &sshConn{Conn: local, cmd: cmd}
	c.copies.Add(2)
	go func() { defer c.copies.Done(); _, _ = io.Copy(in, bridge); _ = in.Close(); _ = bridge.Close() }()
	go func() { defer c.copies.Done(); _, _ = io.Copy(bridge, out); _ = bridge.Close() }()
	if err = ctx.Err(); err != nil {
		_ = c.Close()
		return nil, err
	}
	return c, nil
}

var _ net.Conn = (*sshConn)(nil)
