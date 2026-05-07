package lua

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	glua "github.com/yuin/gopher-lua"
)

func TestValidateModuleName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{name: "simple name", input: "commands"},
		{name: "dotted name", input: "commands.hello"},
		{name: "deeply nested", input: "a.b.c.d"},
		{name: "empty", input: "", wantErr: "cannot be empty"},
		{name: "forward slash", input: "commands/hello", wantErr: "must use dot notation"},
		{name: "backslash", input: `commands\hello`, wantErr: "must use dot notation"},
		{name: "leading dot", input: ".hello", wantErr: "is invalid"},
		{name: "trailing dot", input: "hello.", wantErr: "is invalid"},
		{name: "double dot", input: "a..b", wantErr: "is invalid"},
		{name: "parent traversal", input: "..", wantErr: "is invalid"},
		{name: "current dir", input: ".", wantErr: "is invalid"},
		{name: "relative parent", input: "a..b.c", wantErr: "is invalid"},
		{name: "embedded parent segment", input: "a.b...c", wantErr: "is invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateModuleName(tt.input)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestRequireRejectsPathTraversal(t *testing.T) {
	root := t.TempDir()
	entry := filepath.Join(root, "init.lua")
	require.NoError(t, os.WriteFile(entry, []byte(`
local ok, slashErr = pcall(require, "../escape")
if ok then error("expected require to reject path separators") end

ok, dotErr = pcall(require, "a..b")
if ok then error("expected require to reject parent traversal segments") end

return function(hive)
  hive.commands({ Ok = { sh = "echo ok", help = slashErr .. "|" .. dotErr } })
end
`), 0o644))

	plugin := NewConfigPlugin(entry)
	require.NoError(t, plugin.Init(context.Background()))
	t.Cleanup(func() { require.NoError(t, plugin.Close()) })

	help := plugin.Commands()["Ok"].Help
	assert.Contains(t, help, "must use dot notation")
	assert.Contains(t, help, "is invalid")
}

// newBareRuntime is a Runtime with no host modules — for dispatcher tests.
func newBareRuntime(t *testing.T) *Runtime {
	t.Helper()
	rt, err := NewRuntime(t.TempDir())
	require.NoError(t, err)
	return rt
}

func TestRuntimeSubmitSerializesConcurrentCalls(t *testing.T) {
	t.Parallel()

	rt := newBareRuntime(t)
	t.Cleanup(rt.Close)

	// Non-atomic increments are safe iff the dispatcher serializes
	// closures; -race catches a regression.
	const n = 50 // < dispatcherQueueSize (64), no drops.
	var counter int
	var wg sync.WaitGroup
	wg.Add(n)

	// Barrier so every goroutine races Submit at roughly the same instant.
	var start sync.WaitGroup
	start.Add(1)

	for range n {
		go func() {
			start.Wait()
			rt.Submit(func(_ *glua.LState) {
				counter++
				wg.Done()
			})
		}()
	}
	start.Done()

	doneCh := make(chan struct{})
	go func() {
		wg.Wait()
		close(doneCh)
	}()

	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
		t.Fatalf("dispatcher did not process all Submits within 2s (counter=%d)", counter)
	}

	assert.Equal(t, n, counter, "every submitted closure should have run exactly once")
}

func TestRuntimeSubmitAfterCloseIsNoOp(t *testing.T) {
	t.Parallel()

	rt := newBareRuntime(t)
	rt.Close()

	// Twice to exercise the idempotent post-Close fast path.
	require.NotPanics(t, func() {
		rt.Submit(func(_ *glua.LState) { t.Fatalf("closure must not run after Close") })
		rt.Submit(func(_ *glua.LState) { t.Fatalf("closure must not run after Close") })
	})

	time.Sleep(50 * time.Millisecond)
}

func TestRuntimeSubmitOnNilReceiverIsNoOp(t *testing.T) {
	t.Parallel()

	var rt *Runtime
	require.NotPanics(t, func() {
		rt.Submit(func(_ *glua.LState) {})
	})
}

func TestLoadEntrypointRejectsInvalidArity(t *testing.T) {
	tests := []struct {
		name   string
		script string
		errMsg string
	}{
		{
			name:   "non-function value",
			script: `return {}`,
			errMsg: "must return a function",
		},
		{
			name:   "no return value",
			script: `local _ = 1`,
			errMsg: "must return exactly one function",
		},
		{
			name:   "multiple return values",
			script: `return 1, 2`,
			errMsg: "must return exactly one function",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := filepath.Join(t.TempDir(), "init.lua")
			require.NoError(t, os.WriteFile(entry, []byte(tt.script), 0o644))

			plugin := NewConfigPlugin(entry)
			err := plugin.Init(context.Background())
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errMsg)
		})
	}
}
