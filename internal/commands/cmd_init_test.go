package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestDetectShell covers all shell detection branches.
func TestDetectShell(t *testing.T) {
	tests := []struct {
		name          string
		shellEnv      string
		createProfile bool // create .bash_profile in tmpDir
		wantName      string
		wantRCSuffix  string // suffix of the expected rc file path
	}{
		{
			name:         "zsh",
			shellEnv:     "/usr/bin/zsh",
			wantName:     "zsh",
			wantRCSuffix: ".zshrc",
		},
		{
			name:          "bash with .bash_profile",
			shellEnv:      "/usr/bin/bash",
			createProfile: true,
			wantName:      "bash",
			wantRCSuffix:  ".bash_profile",
		},
		{
			name:         "bash without .bash_profile",
			shellEnv:     "/usr/bin/bash",
			wantName:     "bash",
			wantRCSuffix: ".bashrc",
		},
		{
			name:         "fish",
			shellEnv:     "/usr/bin/fish",
			wantName:     "fish",
			wantRCSuffix: filepath.Join(".config", "fish", "config.fish"),
		},
		{
			name:         "SHELL empty",
			shellEnv:     "",
			wantName:     "unknown",
			wantRCSuffix: "",
		},
		{
			name:         "unknown shell tcsh",
			shellEnv:     "/usr/bin/tcsh",
			wantName:     "unknown",
			wantRCSuffix: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			t.Setenv("HOME", tmpDir)
			t.Setenv("SHELL", tt.shellEnv)

			if tt.createProfile {
				require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".bash_profile"), []byte{}, 0o644))
			}

			gotName, gotRC := detectShell()
			assert.Equal(t, tt.wantName, gotName)

			if tt.wantRCSuffix == "" {
				assert.Empty(t, gotRC)
			} else {
				assert.Equal(t, filepath.Join(tmpDir, tt.wantRCSuffix), gotRC)
			}
		})
	}
}

// TestAliasAlreadyPresent covers file-exists, alias-present, and file-missing cases.
func TestAliasAlreadyPresent(t *testing.T) {
	t.Run("file contains alias hv", func(t *testing.T) {
		f, err := os.CreateTemp(t.TempDir(), "rc-*")
		require.NoError(t, err)
		_, err = f.WriteString("alias hv='tmux new-session -As hive hive'\n")
		require.NoError(t, err)
		require.NoError(t, f.Close())

		present, err := aliasAlreadyPresent(f.Name(), "hv")
		require.NoError(t, err)
		assert.True(t, present)
	})

	t.Run("file without alias hv", func(t *testing.T) {
		f, err := os.CreateTemp(t.TempDir(), "rc-*")
		require.NoError(t, err)
		_, err = f.WriteString("export PATH=$PATH:/usr/local/bin\n")
		require.NoError(t, err)
		require.NoError(t, f.Close())

		present, err := aliasAlreadyPresent(f.Name(), "hv")
		require.NoError(t, err)
		assert.False(t, present)
	})

	t.Run("nonexistent file", func(t *testing.T) {
		present, err := aliasAlreadyPresent(filepath.Join(t.TempDir(), "no-such-file"), "hv")
		require.NoError(t, err)
		assert.False(t, present)
	})
}

// TestAppendAlias verifies that the correct alias syntax is written for each shell.
func TestAppendAlias(t *testing.T) {
	t.Run("zsh appends alias with equals sign", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), ".zshrc")
		require.NoError(t, appendAlias(path, "zsh"))

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Contains(t, string(data), "alias hv=")
	})

	t.Run("fish appends alias with space (no equals)", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.fish")
		require.NoError(t, appendAlias(path, "fish"))

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		content := string(data)
		assert.Contains(t, content, "alias hv ")
		assert.NotContains(t, content, "alias hv=")
	})

	t.Run("single call writes exactly one alias entry", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), ".zshrc")
		require.NoError(t, appendAlias(path, "zsh"))

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, 1, strings.Count(string(data), "alias hv"))
	})
}

// TestDetectInstalledAgents verifies PATH look-up behaviour.
func TestDetectInstalledAgents(t *testing.T) {
	t.Run("unknown binary returns empty slice", func(t *testing.T) {
		result := detectInstalledAgents([]string{"nonexistent-binary-xyz-abc"})
		assert.NotNil(t, result, "result should be non-nil empty slice")
		assert.Empty(t, result)
	})

	t.Run("sh is always on PATH", func(t *testing.T) {
		result := detectInstalledAgents([]string{"sh"})
		assert.Equal(t, []string{"sh"}, result)
	})

	t.Run("order preserved, missing entries filtered", func(t *testing.T) {
		result := detectInstalledAgents([]string{"sh", "nonexistent-binary-xyz"})
		assert.Equal(t, []string{"sh"}, result)
	})
}

// TestToStringSlice checks YAML inline sequence serialisation.
func TestToStringSlice(t *testing.T) {
	t.Run("nil input", func(t *testing.T) {
		assert.Equal(t, "[]", toStringSlice(nil))
	})

	t.Run("empty slice", func(t *testing.T) {
		assert.Equal(t, "[]", toStringSlice([]string{}))
	})

	t.Run("single flag", func(t *testing.T) {
		result := toStringSlice([]string{"--foo"})
		assert.True(t, strings.HasPrefix(result, "["), "expected opening bracket, got: %s", result)
		assert.True(t, strings.HasSuffix(result, "]"), "expected closing bracket, got: %s", result)
		assert.Contains(t, result, "--foo")
	})

	t.Run("multiple flags", func(t *testing.T) {
		result := toStringSlice([]string{"--foo", "--bar"})
		assert.True(t, strings.HasPrefix(result, "["), "expected opening bracket, got: %s", result)
		assert.True(t, strings.HasSuffix(result, "]"), "expected closing bracket, got: %s", result)
		assert.Contains(t, result, "--foo")
		assert.Contains(t, result, "--bar")
	})
}

// TestDefaultConfigPath checks XDG_CONFIG_HOME override and fallback.
func TestDefaultConfigPath(t *testing.T) {
	t.Run("XDG_CONFIG_HOME set", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)

		result := defaultConfigPath()
		assert.True(t, strings.HasPrefix(result, tmpDir), "expected path under %s, got %s", tmpDir, result)
		assert.True(t, strings.HasSuffix(result, "config.yaml"), "expected config.yaml suffix, got %s", result)
	})

	t.Run("XDG_CONFIG_HOME unset falls back to ~/.config/hive/config.yaml", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", "")

		result := defaultConfigPath()
		assert.Contains(t, result, filepath.Join(".config", "hive", "config.yaml"))
	})
}

// TestDetectTmuxConfigPath covers the three resolution branches.
func TestDetectTmuxConfigPath(t *testing.T) {
	t.Run("TMUX_CONFIG env var takes precedence", func(t *testing.T) {
		t.Setenv("TMUX_CONFIG", "/tmp/custom.conf")
		assert.Equal(t, "/tmp/custom.conf", detectTmuxConfigPath())
	})

	t.Run("XDG path returned when file exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("TMUX_CONFIG", "")
		t.Setenv("HOME", tmpDir)
		t.Setenv("XDG_CONFIG_HOME", tmpDir)

		xdgPath := filepath.Join(tmpDir, "tmux", "tmux.conf")
		require.NoError(t, os.MkdirAll(filepath.Dir(xdgPath), 0o755))
		require.NoError(t, os.WriteFile(xdgPath, []byte{}, 0o644))

		assert.Equal(t, xdgPath, detectTmuxConfigPath())
	})

	t.Run("falls back to ~/.tmux.conf when XDG path missing", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("TMUX_CONFIG", "")
		t.Setenv("HOME", tmpDir)
		t.Setenv("XDG_CONFIG_HOME", "") // force default ~/.config path (won't exist)

		result := detectTmuxConfigPath()
		assert.Equal(t, filepath.Join(tmpDir, ".tmux.conf"), result)
	})
}

// TestTmuxBindingAlreadyPresent covers file-missing, binding-absent, and binding-present.
func TestTmuxBindingAlreadyPresent(t *testing.T) {
	t.Run("nonexistent file returns false nil", func(t *testing.T) {
		present, err := tmuxBindingAlreadyPresent(filepath.Join(t.TempDir(), "no-such.conf"))
		require.NoError(t, err)
		assert.False(t, present)
	})

	t.Run("file without binding returns false", func(t *testing.T) {
		f, err := os.CreateTemp(t.TempDir(), "tmux-*")
		require.NoError(t, err)
		_, err = f.WriteString("set -g mouse on\n")
		require.NoError(t, err)
		require.NoError(t, f.Close())

		present, err := tmuxBindingAlreadyPresent(f.Name())
		require.NoError(t, err)
		assert.False(t, present)
	})

	t.Run("file with binding returns true", func(t *testing.T) {
		f, err := os.CreateTemp(t.TempDir(), "tmux-*")
		require.NoError(t, err)
		_, err = f.WriteString("bind-key H switch-client -t hive\n")
		require.NoError(t, err)
		require.NoError(t, f.Close())

		present, err := tmuxBindingAlreadyPresent(f.Name())
		require.NoError(t, err)
		assert.True(t, present)
	})
}

// TestAppendTmuxBinding verifies file creation and content.
func TestAppendTmuxBinding(t *testing.T) {
	t.Run("creates file with binding in existing dir", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), ".tmux.conf")
		require.NoError(t, appendTmuxBinding(path))

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Contains(t, string(data), "bind-key H switch-client -t hive")
	})

	t.Run("creates parent directories when missing", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "nested", "dir", "tmux.conf")
		require.NoError(t, appendTmuxBinding(path))

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Contains(t, string(data), "bind-key H switch-client -t hive")
	})

	t.Run("appends to existing file without truncating", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), ".tmux.conf")
		require.NoError(t, os.WriteFile(path, []byte("set -g mouse on\n"), 0o644))

		require.NoError(t, appendTmuxBinding(path))

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		content := string(data)
		assert.Contains(t, content, "set -g mouse on")
		assert.Contains(t, content, "bind-key H switch-client -t hive")
	})
}

// TestRenderConfigTemplate validates template rendering.
func TestRenderConfigTemplate(t *testing.T) {
	tests := []struct {
		name     string
		data     configTemplateData
		wantYAML bool
		contains []string
	}{
		{
			name: "all fields present and output is valid YAML",
			data: configTemplateData{
				Version:      "1",
				Workspace:    "/home/user/projects",
				AgentDefault: "claude",
				AgentFlags:   []string{"--dangerously-skip-permissions"},
			},
			wantYAML: true,
			contains: []string{"version: 1", "/home/user/projects", "claude", "--dangerously-skip-permissions"},
		},
		{
			name: "empty flags renders as []",
			data: configTemplateData{
				Version:      "1",
				Workspace:    "/home/user/projects",
				AgentDefault: "claude",
			},
			contains: []string{"flags: []"},
		},
		{
			name: "workspace with YAML-significant characters produces valid YAML",
			data: configTemplateData{
				Version:      "1",
				Workspace:    "/home/user/my: code",
				AgentDefault: "claude",
			},
			wantYAML: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := renderConfigTemplate(tt.data)
			require.NoError(t, err)

			for _, s := range tt.contains {
				assert.Contains(t, out, s)
			}

			if tt.wantYAML {
				var parsed map[string]any
				require.NoError(t, yaml.Unmarshal([]byte(out), &parsed), "output must be valid YAML:\n%s", out)
				assert.NotNil(t, parsed["version"])
				assert.NotNil(t, parsed["agents"])
				assert.NotNil(t, parsed["workspaces"])
			}
		})
	}
}

// TestPrintSummary verifies output content for all status types and fixHint.
func TestPrintSummary(t *testing.T) {
	t.Run("all step names and details are present", func(t *testing.T) {
		var buf bytes.Buffer
		printSummary(&buf, []stepResult{
			{name: "Shell alias", status: statusDone, detail: "appended to .zshrc"},
			{name: "Config file", status: statusSkipped, detail: "already exists"},
			{name: "Tmux binding", status: statusFailed, detail: "permission denied"},
		})
		out := buf.String()
		assert.Contains(t, out, "Shell alias")
		assert.Contains(t, out, "appended to .zshrc")
		assert.Contains(t, out, "Config file")
		assert.Contains(t, out, "already exists")
		assert.Contains(t, out, "Tmux binding")
		assert.Contains(t, out, "permission denied")
		assert.Contains(t, out, "Setup Summary")
		assert.Contains(t, out, "hive doctor")
	})

	t.Run("fixHint is printed when set", func(t *testing.T) {
		var buf bytes.Buffer
		printSummary(&buf, []stepResult{
			{name: "Shell alias", status: statusFailed, detail: "err", fixHint: "chmod u+w ~/.zshrc"},
		})
		assert.Contains(t, buf.String(), "chmod u+w ~/.zshrc")
	})

	t.Run("empty results prints header and footer", func(t *testing.T) {
		var buf bytes.Buffer
		printSummary(&buf, nil)
		out := buf.String()
		assert.Contains(t, out, "Setup Summary")
		assert.Contains(t, out, "hive doctor")
	})
}
