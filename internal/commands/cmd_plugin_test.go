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
// and runs `hive plugin init <args...>`. The success message goes to os.Stderr in
// the implementation, so callers should rely on filesystem state for assertions.
func runPluginInit(t *testing.T, args ...string) error {
	t.Helper()
	cmd := NewPluginCmd(&Flags{}, &hive.App{})
	app := &cli.Command{Name: "hive", Writer: &bytes.Buffer{}, ErrWriter: &bytes.Buffer{}}
	cmd.Register(app)
	full := append([]string{"hive", "plugin", "init"}, args...)
	return app.Run(context.Background(), full)
}

func TestPluginInit_NameValidation(t *testing.T) {
	validCases := []string{"my-plugin", "plugin", "a", "a1", "a-b-c"}
	for _, name := range validCases {
		t.Run("valid/"+name, func(t *testing.T) {
			target := filepath.Join(t.TempDir(), "out")
			err := runPluginInit(t, name, "--path", target)
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
			err := runPluginInit(t, args...)
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

	err := runPluginInit(t, "demo")
	require.NoError(t, err)

	initPath := filepath.Join(home, ".config", "hive", "plugins", "demo", "init.lua")
	helloPath := filepath.Join(home, ".config", "hive", "plugins", "demo", "commands", "hello.lua")
	assert.FileExists(t, initPath)
	assert.FileExists(t, helloPath)
}

func TestPluginInit_CustomPath(t *testing.T) {
	target := filepath.Join(t.TempDir(), "foo")
	err := runPluginInit(t, "demo", "--path", target)
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(target, "init.lua"))
	assert.FileExists(t, filepath.Join(target, "commands", "hello.lua"))
}

func TestPluginInit_RefusesExisting(t *testing.T) {
	target := filepath.Join(t.TempDir(), "demo")
	require.NoError(t, os.MkdirAll(target, 0o755))

	err := runPluginInit(t, "demo", "--path", target)
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

	err := runPluginInit(t, "demo", "--path", target, "--force")
	require.NoError(t, err)

	contents, err := os.ReadFile(sentinelPath)
	require.NoError(t, err)
	assert.NotContains(t, string(contents), "SENTINEL", "init.lua should have been overwritten")
	assert.FileExists(t, filepath.Join(target, "commands", "hello.lua"))
}

func TestPluginInit_RenderedContent(t *testing.T) {
	target := filepath.Join(t.TempDir(), "out")
	err := runPluginInit(t, "my-plugin", "--path", target)
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
