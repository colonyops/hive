package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_HIVEDEFAULTAGENTOverridesConfiguredDefault(t *testing.T) {
	t.Setenv(EnvDefaultAgent, "aider")

	configFile := writeConfigFile(t, `
agents:
  default: claude
  claude: {}
  aider:
    command: /opt/bin/aider
`)

	cfg, err := Load(configFile, t.TempDir())
	require.NoError(t, err)

	assert.Equal(t, "aider", cfg.Agents.Default)
	assert.Equal(t, "/opt/bin/aider", cfg.Agents.DefaultProfile().Command)
}

func TestLoad_HIVEDEFAULTAGENTUnsetPreservesConfiguredDefault(t *testing.T) {
	unsetEnv(t, EnvDefaultAgent)

	configFile := writeConfigFile(t, `
agents:
  default: claude
  claude: {}
  aider: {}
`)

	cfg, err := Load(configFile, t.TempDir())
	require.NoError(t, err)

	assert.Equal(t, "claude", cfg.Agents.Default)
}

func TestLoad_HIVEDEFAULTAGENTEmptyPreservesConfiguredDefault(t *testing.T) {
	t.Setenv(EnvDefaultAgent, "")

	configFile := writeConfigFile(t, `
agents:
  default: claude
  claude: {}
  aider: {}
`)

	cfg, err := Load(configFile, t.TempDir())
	require.NoError(t, err)

	assert.Equal(t, "claude", cfg.Agents.Default)
}

func TestLoad_HIVEDEFAULTAGENTUnknownProfileFailsValidation(t *testing.T) {
	t.Setenv(EnvDefaultAgent, "missing")

	configFile := writeConfigFile(t, `
agents:
  default: claude
  claude: {}
`)

	_, err := Load(configFile, t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agents.default")
	assert.Contains(t, err.Error(), `profile "missing" not found`)
}

func TestLoad_HIVECONTEXTBASEDIROverridesConfiguredBaseDir(t *testing.T) {
	t.Setenv(EnvContextBaseDir, "/env/context")

	configFile := writeConfigFile(t, `
context:
  base_dir: /config/context
`)

	cfg, err := Load(configFile, t.TempDir())
	require.NoError(t, err)

	assert.Equal(t, "/env/context", cfg.Context.BaseDir)
	assert.Equal(t, "/env/context", cfg.ContextDir())
}

func TestLoad_HIVEGITPATHOverridesConfiguredGitPath(t *testing.T) {
	t.Setenv(EnvGitPath, "/env/bin/git")

	configFile := writeConfigFile(t, `
git_path: /config/bin/git
`)

	cfg, err := Load(configFile, t.TempDir())
	require.NoError(t, err)

	assert.Equal(t, "/env/bin/git", cfg.GitPath)
}

func TestLoad_EnvironmentOverrideEmptyValuesAreIgnored(t *testing.T) {
	t.Setenv(EnvDefaultAgent, "")
	t.Setenv(EnvContextBaseDir, "")
	t.Setenv(EnvGitPath, "")

	configFile := writeConfigFile(t, `
git_path: /config/bin/git
context:
  base_dir: /config/context
agents:
  default: claude
  claude: {}
  aider: {}
`)

	cfg, err := Load(configFile, t.TempDir())
	require.NoError(t, err)

	assert.Equal(t, "claude", cfg.Agents.Default)
	assert.Equal(t, "/config/context", cfg.Context.BaseDir)
	assert.Equal(t, "/config/bin/git", cfg.GitPath)
}

func writeConfigFile(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()

	value, ok := os.LookupEnv(key)
	require.NoError(t, os.Unsetenv(key))
	t.Cleanup(func() {
		if ok {
			_ = os.Setenv(key, value)
			return
		}
		_ = os.Unsetenv(key)
	})
}
