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
// during plugin init (any hive command triggers it); each function's
// callback records its result, and the final callback writes a summary
// file the test reads back.
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
  -- Chain the calls through nested callbacks so the test sees a single
  -- summary line even though every hive.sh.* call is async.
  hive.sh.run("true", function(runOK)
    hive.sh.run("false", function(runFail)
      hive.sh.output("printf 'hello-output\n'", function(out, err)
        if err ~= nil then
          error("output unexpectedly failed: " .. tostring(err))
        end
        hive.sh.exec("pwd", { cwd = CWD }, function(r)
          local pwd = r.stdout:gsub("\n$", "")
          local summary = string.format(
            "run_ok=%%d|run_fail=%%d|output=%%s|exec_pwd=%%s|exec_code=%%d",
            runOK, runFail, out, pwd, r.code
          )
          hive.sh.run("printf %%s '" .. summary .. "' > " .. OUT, function(_) end)
        end)
      end)
    end)
  end)
end
`, outputFile, cwdMarker)), 0o644))

	h.WithConfig(fmt.Sprintf(`
plugins:
  lua:
    entry: %q
`, entry))

	// Any subcommand triggers plugin init, which runs the entrypoint.
	// Plugin shutdown drains in-flight async work, so the callbacks all
	// run before `hive config` returns.
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

// TestLuaPluginShModule_OutputErrOnNonZero verifies that hive.sh.output
// passes a non-nil err string to its callback for non-zero exits, and
// that the err string mentions the exit code and stderr.
func TestLuaPluginShModule_OutputErrOnNonZero(t *testing.T) {
	h := NewHarness(t)

	outputFile := filepath.Join(h.DataDir(), "sh-output.txt")

	entry := filepath.Join(h.DataDir(), "lua", "plugins", "init.lua")
	require.NoError(t, os.MkdirAll(filepath.Dir(entry), 0o755))
	require.NoError(t, os.WriteFile(entry, []byte(fmt.Sprintf(`
local OUT = %q

return function(hive)
  hive.sh.output("sh -c 'echo boom 1>&2; exit 1'", function(stdout, err)
    -- stdout should be nil on failure; err carries the message.
    local got = "stdout=" .. tostring(stdout) .. "|err=" .. tostring(err)
    hive.sh.run("printf %%s '" .. got .. "' > " .. OUT, function(_) end)
  end)
end
`, outputFile)), 0o644))

	h.WithConfig(fmt.Sprintf(`
plugins:
  lua:
    entry: %q
`, entry))

	_, err := h.RunStdout("config")
	require.NoError(t, err)

	content, err := os.ReadFile(outputFile)
	require.NoError(t, err)
	got := string(content)
	assert.Contains(t, got, "stdout=nil", "non-zero exit should pass nil stdout to the callback")
	assert.Contains(t, got, "exit 1", "err string should mention the exit code")
	assert.Contains(t, got, "boom", "err string should include stderr")
}
