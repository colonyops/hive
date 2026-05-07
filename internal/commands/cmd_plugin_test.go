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

func TestPluginInit_NameValidation(t *testing.T) {
	validCases := []string{"my-plugin", "plugin", "a", "a1", "a-b-c"}
	for _, name := range validCases {
		t.Run("valid/"+name, func(t *testing.T) {
			target := filepath.Join(t.TempDir(), "out")
			_, err := runPluginInit(t, name, "--path", target)
			require.NoError(t, err, "expected name %q to be valid", name)
			assert.FileExists(t, filepath.Join(target, "init.lua"))
			assert.FileExists(t, filepath.Join(target, "commands", "hello.lua"))
		})
	}

	invalidCases := []struct {
		name        string
		input       []string // args after `init`
		errContains []string // any one of these substrings must be present
	}{
		{
			name:        "empty",
			input:       []string{},
			errContains: []string{"argument", "name"},
		},
		{
			name:        "leading-digit",
			input:       []string{"1plugin"},
			errContains: []string{"plugin name", "name"},
		},
		{
			name:  "leading-dash",
			input: []string{"-plugin"},
			// urfave/cli intercepts a leading-dash arg as a flag and reports
			// "flag provided but not defined" before runInit's validator runs.
			// Either failure mode is acceptable: the plugin is not created.
			errContains: []string{"plugin name", "name", "flag provided but not defined"},
		},
		{
			name:        "uppercase",
			input:       []string{"My-Plugin"},
			errContains: []string{"plugin name", "name"},
		},
		{
			name:        "underscore",
			input:       []string{"my_plugin"},
			errContains: []string{"plugin name", "name"},
		},
		{
			name:        "slash",
			input:       []string{"my/plugin"},
			errContains: []string{"plugin name", "name"},
		},
		{
			name:        "double-dot",
			input:       []string{".."},
			errContains: []string{"plugin name", "name"},
		},
		{
			name:        "embedded-dots",
			input:       []string{"my..plugin"},
			errContains: []string{"plugin name", "name"},
		},
	}
	for _, tc := range invalidCases {
		t.Run("invalid/"+tc.name, func(t *testing.T) {
			// Use a tempdir as the path so even if validation regresses we don't
			// pollute the user's $HOME.
			args := append([]string{}, tc.input...)
			args = append(args, "--path", filepath.Join(t.TempDir(), "out"))
			_, err := runPluginInit(t, args...)
			require.Error(t, err, "expected error for input %v", tc.input)
			msg := err.Error()
			matched := false
			for _, sub := range tc.errContains {
				if strings.Contains(msg, sub) {
					matched = true
					break
				}
			}
			assert.True(t, matched, "error %q did not contain any of %v", msg, tc.errContains)
		})
	}
}

func TestPluginInit_DefaultPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")

	_, err := runPluginInit(t, "demo")
	require.NoError(t, err)

	initPath := filepath.Join(home, ".config", "hive", "plugins", "demo", "init.lua")
	helloPath := filepath.Join(home, ".config", "hive", "plugins", "demo", "commands", "hello.lua")
	assert.FileExists(t, initPath)
	assert.FileExists(t, helloPath)
}

func TestPluginInit_CustomPath(t *testing.T) {
	target := filepath.Join(t.TempDir(), "foo")
	_, err := runPluginInit(t, "demo", "--path", target)
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(target, "init.lua"))
	assert.FileExists(t, filepath.Join(target, "commands", "hello.lua"))
}

func TestPluginInit_RefusesExisting(t *testing.T) {
	target := filepath.Join(t.TempDir(), "demo")
	require.NoError(t, os.MkdirAll(target, 0o755))

	_, err := runPluginInit(t, "demo", "--path", target)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--force")

	_, statErr := os.Stat(filepath.Join(target, "init.lua"))
	assert.True(t, os.IsNotExist(statErr), "init.lua should not have been written")
}

func TestPluginInit_ForceOverwrites(t *testing.T) {
	target := filepath.Join(t.TempDir(), "demo")
	require.NoError(t, os.MkdirAll(target, 0o755))
	sentinelPath := filepath.Join(target, "init.lua")
	require.NoError(t, os.WriteFile(sentinelPath, []byte("-- SENTINEL"), 0o644))

	_, err := runPluginInit(t, "demo", "--path", target, "--force")
	require.NoError(t, err)

	contents, err := os.ReadFile(sentinelPath)
	require.NoError(t, err)
	assert.NotContains(t, string(contents), "SENTINEL", "init.lua should have been overwritten")
	assert.FileExists(t, filepath.Join(target, "commands", "hello.lua"))
}

func TestPluginInit_RenderedContent(t *testing.T) {
	target := filepath.Join(t.TempDir(), "out")
	_, err := runPluginInit(t, "my-plugin", "--path", target)
	require.NoError(t, err)

	initBytes, err := os.ReadFile(filepath.Join(target, "init.lua"))
	require.NoError(t, err)
	initContent := string(initBytes)
	assert.Contains(t, initContent, "my-plugin", "init.lua should contain the kebab name")
	assert.Contains(t, initContent, "MyPluginHello", "init.lua should contain the PascalCased command-prefixed key")

	helloBytes, err := os.ReadFile(filepath.Join(target, "commands", "hello.lua"))
	require.NoError(t, err)
	helloContent := string(helloBytes)
	assert.True(t,
		strings.Contains(helloContent, "M.greet") || strings.Contains(helloContent, "function M.greet"),
		"hello.lua should reference M.greet, got: %s", helloContent)
}

// TestPluginInit_SuccessMessage asserts the activation banner advertises only
// the entry-file activation path and contains the resolved init.lua location.
// The kebab-case "require()-from-top-level" snippet was dropped because
// require("commands.hello") only resolves when the scaffolded init.lua is
// itself the entry — so the banner must not promise a second mode that fails
// at runtime.
func TestPluginInit_SuccessMessage(t *testing.T) {
	target := filepath.Join(t.TempDir(), "out")
	stderr, err := runPluginInit(t, "my-plugin", "--path", target)
	require.NoError(t, err)

	assert.Contains(t, stderr, "Scaffolded plugin")
	assert.Contains(t, stderr, filepath.Join(target, "init.lua"))
	assert.Contains(t, stderr, "entry:", "banner should show the plugins.lua.entry activation hint")
	// The previous banner advertised a broken `require("<name>.init")` path.
	// Make sure that wording does not return.
	assert.NotContains(t, stderr, "require(", "banner must not advertise the broken require()-based activation")
	assert.NotContains(t, stderr, "top-level", "banner must not reference top-level init.lua activation")
}

// TestPluginInit_StatErrorSurfaced ensures non-IsNotExist stat errors surface
// the real cause (wrapping the OS error) instead of the misleading
// "already exists" message that the previous branch returned.
func TestPluginInit_StatErrorSurfaced(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses directory permission checks")
	}
	parent := t.TempDir()
	require.NoError(t, os.Chmod(parent, 0o000))
	t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })

	target := filepath.Join(parent, "demo")
	_, err := runPluginInit(t, "demo", "--path", target)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stat ", "error should report the failed operation")
	assert.NotContains(t, err.Error(), "already exists", "must not lie about existence on permission errors")
}

// TestPluginInit_TwoArgs asserts that `init` rejects more than one positional
// argument; the validator's Args().Len() != 1 check would silently accept
// extras if it regressed to "< 1".
func TestPluginInit_TwoArgs(t *testing.T) {
	target := filepath.Join(t.TempDir(), "out")
	_, err := runPluginInit(t, "foo", "bar", "--path", target)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one")
	_, statErr := os.Stat(filepath.Join(target, "init.lua"))
	assert.True(t, os.IsNotExist(statErr), "no files should have been written")
}
