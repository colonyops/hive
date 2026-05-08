//go:build integration

package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLuaPluginShModule_RunOutputExec verifies that hive.sh.{run,output,exec}
// are wired through the Lua runtime end-to-end. The plugin's entrypoint runs
// during plugin init (any hive command triggers it); each function records
// its result via a final hive.sh.run that writes a single summary file the
// test reads back.
func TestLuaPluginShModule_RunOutputExec(t *testing.T) {
	h := NewHarness(t)

	outputFile := filepath.Join(h.DataDir(), "sh-output.txt")
	cwdMarker := filepath.Join(h.DataDir(), "sh-cwd-marker")
	require.NoError(t, os.MkdirAll(cwdMarker, 0o755))

	entry := filepath.Join(h.DataDir(), "lua", "plugins", "init.lua")
	require.NoError(t, os.MkdirAll(filepath.Dir(entry), 0o755))
	require.NoError(t, os.WriteFile(entry, []byte(fmt.Sprintf(`
local OUT = %q
local CWD = %q

return function(hive)
  -- run: returns exit code only, never raises
  local runOK   = hive.sh.run("true")
  local runFail = hive.sh.run("false")

  -- output: captures stdout, strips trailing newline
  local out = hive.sh.output("printf 'hello-output\n'")

  -- exec: full struct with cwd option (trim trailing newline so it slots
  -- cleanly into the summary line)
  local r = hive.sh.exec("pwd", { cwd = CWD })
  local pwd = r.stdout:gsub("\n$", "")

  local summary = string.format(
    "run_ok=%%d|run_fail=%%d|output=%%s|exec_pwd=%%s|exec_code=%%d",
    runOK, runFail, out, pwd, r.code
  )
  hive.sh.run("printf %%s '" .. summary .. "' > " .. OUT)
end
`, outputFile, cwdMarker)), 0o644))

	h.WithConfig(fmt.Sprintf(`
plugins:
  lua:
    entry: %q
`, entry))

	// Any subcommand triggers plugin init, which runs the entrypoint.
	_, err := h.RunStdout("config")
	require.NoError(t, err)

	content, err := os.ReadFile(outputFile)
	require.NoError(t, err, "summary file should be created by the lua plugin entrypoint")

	got := string(content)
	assert.Contains(t, got, "run_ok=0", "hive.sh.run should report exit code 0 for 'true'")
	assert.Contains(t, got, "run_fail=1", "hive.sh.run should report exit code 1 for 'false'")
	assert.Contains(t, got, "output=hello-output", "hive.sh.output should capture stdout and strip trailing newline")
	// pwd from exec should resolve to CWD (or its symlink-resolved form on macOS).
	resolvedCWD, err := filepath.EvalSymlinks(cwdMarker)
	require.NoError(t, err)
	assert.True(t,
		strings.Contains(got, cwdMarker) || strings.Contains(got, resolvedCWD),
		"exec_pwd should reflect opts.cwd; got: %s", got)
	assert.Contains(t, got, "exec_code=0", "hive.sh.exec should report exit code 0")
}

// TestLuaPluginShModule_OutputRaisesOnNonZero verifies that hive.sh.output
// raises a Lua error for non-zero exits, and that the error is logged when
// it propagates out of the entrypoint without crashing hive.
func TestLuaPluginShModule_OutputRaisesOnNonZero(t *testing.T) {
	h := NewHarness(t)

	entry := filepath.Join(h.DataDir(), "lua", "plugins", "init.lua")
	require.NoError(t, os.MkdirAll(filepath.Dir(entry), 0o755))
	require.NoError(t, os.WriteFile(entry, []byte(`
return function(hive)
  hive.sh.output("sh -c 'echo boom 1>&2; exit 1'")
end
`), 0o644))

	h.WithConfig(fmt.Sprintf(`
plugins:
  lua:
    entry: %q
`, entry))

	// hive should still run even though the lua plugin's entrypoint errored.
	_, err := h.RunStdout("config")
	require.NoError(t, err)

	logContent, err := os.ReadFile(filepath.Join(h.DataDir(), "hive.log"))
	require.NoError(t, err)
	logStr := string(logContent)
	assert.Contains(t, logStr, "plugin initialization failed")
	assert.Contains(t, logStr, "hive.sh.output")
	assert.Contains(t, logStr, "exit 1")
}

// TestLuaPluginShModule_AsyncDoesNotBlockTicker verifies that the async form
// of hive.sh.run returns immediately and does not block the Lua dispatcher.
//
// The original plan called for a ticker.every fire-pattern across multiple
// seconds, but `hive config` tears the plugin down immediately after init,
// so we instead prove the property within a single entrypoint: launching a
// long async sleep MUST NOT delay the synchronous work that follows.
//
// We can't time the property from the host side: hive's plugin shutdown
// drains in-flight async work, so the total `hive config` wall time always
// includes the residual sleep. Instead the entrypoint stamps "start" and
// "done" marker files around the sync block. The mtime delta between the
// two markers is the entrypoint wall time, isolated from shutdown drain.
// With async dispatching correctly, that delta is ~2s (only the two 1s
// sync sleeps). If async incorrectly blocked the dispatcher, the delta
// would jump to ~7s (the 5s blocking sleep plus the two 1s sync sleeps).
func TestLuaPluginShModule_AsyncDoesNotBlockTicker(t *testing.T) {
	h := NewHarness(t)

	counter := filepath.Join(h.DataDir(), "counter.txt")
	startMarker := filepath.Join(h.DataDir(), "start.marker")
	doneMarker := filepath.Join(h.DataDir(), "done.marker")

	entry := filepath.Join(h.DataDir(), "lua", "plugins", "init.lua")
	require.NoError(t, os.MkdirAll(filepath.Dir(entry), 0o755))
	require.NoError(t, os.WriteFile(entry, []byte(fmt.Sprintf(`
local COUNTER = %q
local START   = %q
local DONE    = %q

return function(hive)
  -- Stamp the start marker just before kicking off the async work so the
  -- mtime delta between START and DONE captures only the entrypoint's
  -- wall time, not hive's startup/shutdown overhead.
  hive.sh.run("touch " .. START)

  -- Async: kicks off a 5s sleep. If async-doesn't-block-the-dispatcher
  -- holds, this returns immediately and the rest of the entrypoint runs
  -- in ~2s wall time. The async subprocess is cancelled when the plugin
  -- shuts down at the end of the hive command.
  hive.sh.run("sleep 5", function(_) end)

  hive.sh.run("printf 'tick1\n' >> " .. COUNTER)
  hive.sh.run("sleep 1")
  hive.sh.run("printf 'tick2\n' >> " .. COUNTER)
  hive.sh.run("sleep 1")
  hive.sh.run("printf 'tick3\n' >> " .. COUNTER)

  hive.sh.run("touch " .. DONE)
end
`, counter, startMarker, doneMarker)), 0o644))

	h.WithConfig(fmt.Sprintf(`
plugins:
  lua:
    entry: %q
`, entry))

	_, err := h.RunStdout("config")
	require.NoError(t, err)

	// Counter should hold three ticks in order, proving the sync sleeps
	// and writes ran sequentially after the async launch.
	content, err := os.ReadFile(counter)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimRight(string(content), "\n"), "\n")
	assert.Equal(t, []string{"tick1", "tick2", "tick3"}, lines,
		"counter should record three ticks in order; got %q", string(content))

	startInfo, err := os.Stat(startMarker)
	require.NoError(t, err, "start marker should exist")
	doneInfo, err := os.Stat(doneMarker)
	require.NoError(t, err, "done marker should exist")

	entrypointDelta := doneInfo.ModTime().Sub(startInfo.ModTime())
	t.Logf("entrypoint wall time (start->done marker delta): %s", entrypointDelta)

	// Sync work alone is ~2s; the entrypoint should fit comfortably under
	// 4s if async did not block the dispatcher. If async incorrectly
	// blocked, the delta would also include the 5s sleep, pushing it past
	// 7s. 4s leaves generous margin for filesystem mtime granularity and
	// CI noise.
	assert.Less(t, entrypointDelta, 4*time.Second,
		"entrypoint took %s between start and done markers — async hive.sh.run appears to block the dispatcher",
		entrypointDelta)
}
