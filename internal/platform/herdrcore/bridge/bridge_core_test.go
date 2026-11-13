package bridge_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"

	"github.com/colonyops/hive/internal/platform/herdrcore"
	"github.com/colonyops/hive/internal/platform/herdrcore/bridge"
	"github.com/colonyops/hive/internal/platform/herdrcore/hctest"
)

// The test binary doubles as the bridge executable.
func TestMain(m *testing.M) {
	if code, ok := bridge.Main(os.Args); ok {
		os.Exit(code)
	}
	os.Exit(m.Run())
}

func TestBridgeRelaysInputOutputAndSize(t *testing.T) {
	socket := hctest.Start(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	c, err := herdrcore.Dial(ctx, socket)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	term, err := c.Run(ctx, herdrcore.RunParams{Argv: []string{"/bin/cat"}, NoParent: true})
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(os.Args[0], bridge.Flag, socket, term.Address)
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: 90, Rows: 20})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ptmx.Close() }()

	var mu sync.Mutex
	var seen bytes.Buffer
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			mu.Lock()
			seen.Write(buf[:n])
			mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	waitUntil(t, "the bridge attached at the PTY's size", func() bool {
		got, err := c.Get(ctx, term.Address)
		return err == nil && got.Cols == 90 && got.Rows == 20
	})

	if _, err := ptmx.WriteString("over-the-bridge\r"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "cat's echo through the bridge", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return strings.Contains(seen.String(), "over-the-bridge")
	})

	if err := pty.Setsize(ptmx, &pty.Winsize{Cols: 120, Rows: 33}); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "a PTY resize to reach the core", func() bool {
		got, err := c.Get(ctx, term.Address)
		return err == nil && got.Cols == 120 && got.Rows == 33
	})

	if err := c.Kill(ctx, term.Address); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	select {
	case <-exited:
	case <-time.After(10 * time.Second):
		t.Fatal("the bridge kept running after its terminal ended")
	}
}

func waitUntil(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// Vim leaves insert mode on a lone Esc. Held as the possible start of a
// sequence, it must still go out once nothing follows it.
func TestBridgeSendsALoneEscape(t *testing.T) {
	socket := hctest.Start(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c, err := herdrcore.Dial(ctx, socket)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	term, err := c.Run(ctx, herdrcore.RunParams{Argv: []string{"/bin/cat", "-v"}, NoParent: true})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], bridge.Flag, socket, term.Address)
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: 80, Rows: 10})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ptmx.Close() }()
	go func() { _, _ = io.Copy(io.Discard, ptmx) }()

	time.Sleep(time.Second)
	if _, err := ptmx.WriteString("\x1b"); err != nil {
		t.Fatal(err)
	}
	// The PTY echoes a received Esc as ^[.
	waitUntil(t, "the lone Esc to reach the program", func() bool {
		var read struct {
			Text string `json:"text"`
		}
		err := c.Call(ctx, "terminal.read", map[string]any{"terminal": term.Address}, &read)
		return err == nil && strings.Contains(read.Text, "^[")
	})
}
