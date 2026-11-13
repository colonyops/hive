package herdrcore_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/platform/herdrcore"
	"github.com/colonyops/hive/internal/platform/herdrcore/hctest"
)

func TestRunAttachTypeAndRead(t *testing.T) {
	socket := hctest.Start(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	c, err := herdrcore.Dial(ctx, socket)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	if c.Hello().Generation != herdrcore.Generation {
		t.Fatalf("generation %d", c.Hello().Generation)
	}

	term, err := c.Run(ctx, herdrcore.RunParams{Argv: []string{"/bin/cat"}, Size: &herdrcore.Size{Cols: 80, Rows: 24}, NoParent: true})
	if err != nil {
		t.Fatal(err)
	}
	s, res, err := c.Attach(ctx, herdrcore.AttachParams{Terminal: term.Address, Cols: 100, Rows: 30})
	if err != nil {
		t.Fatal(err)
	}
	if res.Terminal.Address != term.Address {
		t.Fatalf("attached %s, want %s", res.Terminal.Address, term.Address)
	}
	if _, err := s.Write([]byte("hello-herdr\r")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, s, "hello-herdr")

	got, err := c.Get(ctx, term.Address)
	if err != nil {
		t.Fatal(err)
	}
	if got.Cols != 100 || got.Rows != 30 {
		t.Errorf("the active viewer sets the size: got %dx%d", got.Cols, got.Rows)
	}

	if err := c.Kill(ctx, term.Address); err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(io.Discard, s); err != nil && !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
	if r := s.Ended(); r == nil || r.Reason != "killed" {
		t.Errorf("close reason = %+v, want killed", r)
	}

	_, err = c.Get(ctx, "t999")
	if rpcErr, ok := errors.AsType[*herdrcore.RPCError](err); !ok || rpcErr.Kind() != "not_found" {
		t.Errorf("unknown terminal: %v", err)
	}
}

func waitFor(t *testing.T, r io.Reader, want string) {
	t.Helper()
	found := make(chan struct{})
	var seen bytes.Buffer
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			seen.Write(buf[:n])
			if strings.Contains(seen.String(), want) {
				close(found)
				return
			}
			if err != nil {
				return
			}
		}
	}()
	select {
	case <-found:
	case <-time.After(10 * time.Second):
		t.Fatalf("never saw %q in %q", want, seen.String())
	}
}
