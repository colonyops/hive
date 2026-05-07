package commands

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/colonyops/hive/internal/hive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

// runPluginInit constructs a fresh PluginCmd, registers it on a new cli.Command,
// and runs `hive plugin init <args...>`. The captured stderr buffer holds the
// success banner so callers can assert on its contents.
func runPluginInit(t *testing.T, args ...string) (stderr string, err error) {
	t.Helper()
	cmd := NewPluginCmd(&Flags{}, &hive.App{})
	var errBuf bytes.Buffer
	app := &cli.Command{Name: "hive", Writer: &bytes.Buffer{}, ErrWriter: &errBuf}
	cmd.Register(app)
	full := append([]string{"hive", "plugin", "init"}, args...)
	err = app.Run(context.Background(), full)
	return errBuf.String(), err
}

func TestPluginInit_DefaultPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")

	_, err := runPluginInit(t)
	require.NoError(t, err)

	plugins := filepath.Join(home, ".config", "hive", "plugins")
	assert.FileExists(t, filepath.Join(plugins, "init.lua"))
	assert.FileExists(t, filepath.Join(plugins, "commands", "hello.lua"))
}

func TestPluginInit_CustomPath(t *testing.T) {
	target := filepath.Join(t.TempDir(), "myplugin")
	_, err := runPluginInit(t, "--path", target)
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(target, "init.lua"))
	assert.FileExists(t, filepath.Join(target, "commands", "hello.lua"))
}

func TestPluginInit_RefusesExisting(t *testing.T) {
	target := filepath.Join(t.TempDir(), "p")
	require.NoError(t, os.MkdirAll(target, 0o755))
	initPath := filepath.Join(target, "init.lua")
	require.NoError(t, os.WriteFile(initPath, []byte("-- existing"), 0o644))

	_, err := runPluginInit(t, "--path", target)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--force")

	contents, readErr := os.ReadFile(initPath)
	require.NoError(t, readErr)
	assert.Equal(t, "-- existing", string(contents), "init.lua must not be touched without --force")
}

func TestPluginInit_ForceOverwrites(t *testing.T) {
	target := filepath.Join(t.TempDir(), "p")
	require.NoError(t, os.MkdirAll(target, 0o755))
	initPath := filepath.Join(target, "init.lua")
	require.NoError(t, os.WriteFile(initPath, []byte("-- SENTINEL"), 0o644))

	_, err := runPluginInit(t, "--path", target, "--force")
	require.NoError(t, err)

	contents, err := os.ReadFile(initPath)
	require.NoError(t, err)
	assert.NotContains(t, string(contents), "SENTINEL", "init.lua should have been overwritten")
	assert.FileExists(t, filepath.Join(target, "commands", "hello.lua"))
}

func TestPluginInit_RenderedContent(t *testing.T) {
	target := filepath.Join(t.TempDir(), "p")
	_, err := runPluginInit(t, "--path", target)
	require.NoError(t, err)

	initBytes, err := os.ReadFile(filepath.Join(target, "init.lua"))
	require.NoError(t, err)
	initContent := string(initBytes)
	assert.Contains(t, initContent, "LuaHello", "init.lua should register the LuaHello command")
	assert.Contains(t, initContent, `require("commands.hello")`, "init.lua should demonstrate require")

	helloBytes, err := os.ReadFile(filepath.Join(target, "commands", "hello.lua"))
	require.NoError(t, err)
	assert.Contains(t, string(helloBytes), "function M.greet")
}

// TestPluginInit_SuccessMessage asserts the activation banner advertises only
// the entry-file activation path. The kebab-case "require()-from-top-level"
// snippet was dropped because require("commands.hello") only resolves when
// the scaffolded init.lua is itself the entry — so the banner must not
// promise a second mode that fails at runtime.
func TestPluginInit_SuccessMessage(t *testing.T) {
	target := filepath.Join(t.TempDir(), "p")
	stderr, err := runPluginInit(t, "--path", target)
	require.NoError(t, err)

	assert.Contains(t, stderr, "Scaffolded Lua plugin")
	assert.Contains(t, stderr, filepath.Join(target, "init.lua"))
	assert.Contains(t, stderr, "plugins.lua.entry", "banner should mention the activation key")
	// The previous banner advertised a broken `require("<name>.init")` path.
	// Make sure that wording does not return.
	assert.NotContains(t, stderr, `require("`, "banner must not advertise the broken require()-based activation")
	assert.NotContains(t, stderr, "top-level", "banner must not reference top-level init.lua activation")
}

// TestPluginInit_StatErrorSurfaced ensures non-IsNotExist stat errors surface
// the real cause (wrapping the OS error) instead of the misleading
// "already exists" message that an earlier branch returned.
func TestPluginInit_StatErrorSurfaced(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses directory permission checks")
	}
	parent := t.TempDir()
	require.NoError(t, os.Chmod(parent, 0o000))
	t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })

	target := filepath.Join(parent, "p")
	_, err := runPluginInit(t, "--path", target)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stat ", "error should report the failed operation")
	assert.NotContains(t, err.Error(), "already exists", "must not lie about existence on permission errors")
}

// TestPluginInit_RejectsArgs asserts that `init` rejects any positional
// arguments — the legacy form took a <name> arg which is no longer supported.
func TestPluginInit_RejectsArgs(t *testing.T) {
	target := filepath.Join(t.TempDir(), "p")
	_, err := runPluginInit(t, "myname", "--path", target)
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "no positional")
	_, statErr := os.Stat(filepath.Join(target, "init.lua"))
	assert.True(t, os.IsNotExist(statErr), "no files should have been written")
}
