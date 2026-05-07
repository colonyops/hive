//go:build integration

package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
