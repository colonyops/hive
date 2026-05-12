package commands

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/hive"
)

func TestResolvePrompt(t *testing.T) {
	t.Run("direct value returned unchanged", func(t *testing.T) {
		cmd := &TimerCmd{prompt: "hello world"}
		got, err := cmd.resolvePrompt(strings.NewReader("ignored"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "hello world" {
			t.Errorf("got %q, want %q", got, "hello world")
		}
	})

	t.Run("stdin read when flag is dash", func(t *testing.T) {
		cmd := &TimerCmd{prompt: "-"}
		got, err := cmd.resolvePrompt(strings.NewReader("from stdin\n"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "from stdin" {
			t.Errorf("got %q, want %q", got, "from stdin")
		}
	})

	t.Run("trailing CRLF trimmed from stdin", func(t *testing.T) {
		cmd := &TimerCmd{prompt: "-"}
		got, err := cmd.resolvePrompt(strings.NewReader("prompt\r\n"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "prompt" {
			t.Errorf("got %q, want %q", got, "prompt")
		}
	})

	t.Run("stdin read error propagated", func(t *testing.T) {
		cmd := &TimerCmd{prompt: "-"}
		_, err := cmd.resolvePrompt(&errReader{})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

// errReader always returns an error on Read.
type errReader struct{}

func (e *errReader) Read(_ []byte) (int, error) {
	return 0, &testError{msg: "read error"}
}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

func TestPrintConfirmation(t *testing.T) {
	firesAt := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	d := 5 * time.Minute

	t.Run("text format", func(t *testing.T) {
		cmd := &TimerCmd{asJSON: false}
		var buf bytes.Buffer
		if err := cmd.printConfirmation(&buf, "abc123", "sess:2", firesAt, d, 9999); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "abc123") {
			t.Errorf("expected id in output, got %q", out)
		}
		if !strings.Contains(out, "sess:2") {
			t.Errorf("expected target in output, got %q", out)
		}
	})

	t.Run("json format", func(t *testing.T) {
		cmd := &TimerCmd{asJSON: true}
		var buf bytes.Buffer
		if err := cmd.printConfirmation(&buf, "abc123", "sess:2", firesAt, d, 9999); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var result struct {
			ID        string `json:"id"`
			FiresAt   string `json:"fires_at"`
			InSeconds int64  `json:"in_seconds"`
			Target    string `json:"target"`
			PID       int    `json:"pid"`
		}
		if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
			t.Fatalf("unmarshal: %v, raw: %q", err, buf.String())
		}
		if result.ID != "abc123" {
			t.Errorf("id: got %q, want %q", result.ID, "abc123")
		}
		if result.Target != "sess:2" {
			t.Errorf("target: got %q, want %q", result.Target, "sess:2")
		}
		if result.InSeconds != int64(d.Seconds()) {
			t.Errorf("in_seconds: got %d, want %d", result.InSeconds, int64(d.Seconds()))
		}
		if result.PID != 9999 {
			t.Errorf("pid: got %d, want 9999", result.PID)
		}
	})
}

func TestSetEnvKey(t *testing.T) {
	t.Run("replaces existing key", func(t *testing.T) {
		env := []string{"FOO=old", "BAR=baz"}
		got := setEnvKey(env, "FOO", "new")
		if len(got) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(got))
		}
		if got[0] != "FOO=new" {
			t.Errorf("got %q, want %q", got[0], "FOO=new")
		}
	})

	t.Run("appends new key", func(t *testing.T) {
		env := []string{"BAR=baz"}
		got := setEnvKey(env, "NEW", "val")
		if len(got) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(got))
		}
		if got[1] != "NEW=val" {
			t.Errorf("got %q, want %q", got[1], "NEW=val")
		}
	})
}

func TestBuildChildEnv_SkipsEmptyValues(t *testing.T) {
	opts := hive.BootstrapOptions{
		DataDir: "/data",
		LogFile: "",
	}
	env := buildChildEnv(opts)
	for _, e := range env {
		if strings.HasPrefix(e, "HIVE_LOG_FILE=") {
			t.Errorf("expected HIVE_LOG_FILE to be absent when LogFile is empty, got %q", e)
		}
	}
	found := false
	for _, e := range env {
		if e == "HIVE_DATA_DIR=/data" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected HIVE_DATA_DIR=/data in env")
	}
}
