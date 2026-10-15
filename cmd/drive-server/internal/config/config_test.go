package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validConfig() Config {
	return Config{
		BaseURL:         "https://drive.example.com",
		Listen:          ":8080",
		MaxDocumentSize: Size{Bytes: 1 << 20},
		UserPolicies: []Policy{
			{Name: "admins", Matches: []string{"*@example.com"}, Quotas: Quotas{Limit: Size{Unlimited: true}}},
			{Name: "default", Matches: []string{"*"}, Quotas: Quotas{Limit: Size{Bytes: 1 << 30}}},
		},
	}
}

func TestValidateAcceptsValidConfig(t *testing.T) {
	warnings, err := validConfig().Validate()
	require.NoError(t, err)
	assert.Empty(t, warnings)
}

func TestValidateErrors(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{"missing base_url", func(c *Config) { c.BaseURL = "" }, "base_url is required"},
		{"relative base_url", func(c *Config) { c.BaseURL = "drive.example.com" }, "absolute URL"},
		{"http on public host", func(c *Config) { c.BaseURL = "http://drive.example.com" }, "loopback"},
		{"other scheme", func(c *Config) { c.BaseURL = "ftp://drive.example.com" }, "https://"},
		{"duplicate name", func(c *Config) { c.UserPolicies[1].Name = "admins" }, "duplicate name"},
		{"empty matches", func(c *Config) { c.UserPolicies[0].Matches = nil }, "matches is empty"},
		{"bad glob", func(c *Config) { c.UserPolicies[0].Matches = []string{"[a"} }, `bad glob "[a"`},
		{"missing limit", func(c *Config) { c.UserPolicies[0].Quotas = Quotas{} }, "quotas.limit is required"},
		{"smtp password in clear", func(c *Config) { c.SMTP.Password = "hunter2" }, "secret reference"},
		{"smtp bad url", func(c *Config) { c.SMTP.URL = "http://mail" }, "smtp.url"},
		{"unset document size", func(c *Config) { c.MaxDocumentSize = Size{} }, "max_document_size"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.mutate(&cfg)
			_, err := cfg.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestValidateAllowsHTTPOnLoopback(t *testing.T) {
	for _, u := range []string{"http://127.0.0.1:8080", "http://localhost:8080", "http://[::1]:8080"} {
		cfg := validConfig()
		cfg.BaseURL = u
		_, err := cfg.Validate()
		assert.NoError(t, err, u)
	}
}

func TestValidateWarnsOnPolicyBelowCatchAll(t *testing.T) {
	cfg := validConfig()
	cfg.UserPolicies = []Policy{cfg.UserPolicies[1], cfg.UserPolicies[0]}

	warnings, err := cfg.Validate()
	require.NoError(t, err)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "admins")
	assert.Contains(t, warnings[0], `"default"`)
}

func writeConfigFile(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ConfigFileName), []byte(body), 0o600))
	return dir
}

func envOf(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadPrecedence(t *testing.T) {
	dir := writeConfigFile(t, `
base_url: https://file.example.com
listen: ":1111"
data_dir: /file-data
max_document_size: 1MiB
`)
	env := envOf(map[string]string{
		"HIVE_DRIVE_SERVER_LISTEN":   ":2222",
		"HIVE_DRIVE_SERVER_DATA_DIR": "/env-data",
	})

	cfg, err := Load(env, Flags{ConfigDir: dir, DataDir: "/flag-data"})
	require.NoError(t, err)

	assert.Equal(t, "https://file.example.com", cfg.BaseURL, "file applies when env and flags are silent")
	assert.Equal(t, ":2222", cfg.Listen, "env beats file")
	assert.Equal(t, "/flag-data", cfg.DataDir, "flag beats env and file")
	assert.Equal(t, int64(1<<20), cfg.MaxDocumentSize.Bytes)
}

func TestLoadDefaultsAndMissingFile(t *testing.T) {
	for _, dir := range []string{t.TempDir(), filepath.Join(t.TempDir(), "absent")} {
		cfg, err := Load(envOf(nil), Flags{ConfigDir: dir})
		require.NoError(t, err)
		assert.Equal(t, DefaultListen, cfg.Listen)
		assert.Equal(t, DefaultDataDir, cfg.DataDir)
		assert.Equal(t, int64(DefaultMaxDocumentSize), cfg.MaxDocumentSize.Bytes)
		assert.Empty(t, cfg.UserPolicies)
	}
}

func TestLoadEnvConfigDirAndOverrides(t *testing.T) {
	dir := writeConfigFile(t, "base_url: https://file.example.com\n")
	env := envOf(map[string]string{
		"HIVE_DRIVE_SERVER_CONFIG_DIR":        dir,
		"HIVE_DRIVE_SERVER_BASE_URL":          "https://env.example.com/",
		"HIVE_DRIVE_SERVER_MAX_DOCUMENT_SIZE": "2MiB",
		"HIVE_DRIVE_SERVER_TRUSTED_PROXIES":   "10.0.0.0/8, 192.168.0.0/16",
		"HIVE_DRIVE_SERVER_DEV_LOG_MAIL":      "true",
		"HIVE_DRIVE_SERVER_SMTP_URL":          "smtps://mail.example.com",
	})

	cfg, err := Load(env, Flags{})
	require.NoError(t, err)

	assert.Equal(t, "https://env.example.com", cfg.BaseURL)
	assert.Equal(t, int64(2<<20), cfg.MaxDocumentSize.Bytes)
	assert.Equal(t, []string{"10.0.0.0/8", "192.168.0.0/16"}, cfg.TrustedProxies)
	assert.True(t, cfg.DevLogMail)
	assert.Equal(t, "smtps://mail.example.com", cfg.SMTP.URL)
}

func TestLoadPolicies(t *testing.T) {
	dir := writeConfigFile(t, `
user_policies:
  - name: default
    matches: ["*"]
    sign_up: false
    quotas:
      limit: unlimited
`)
	cfg, err := Load(envOf(nil), Flags{ConfigDir: dir})
	require.NoError(t, err)

	require.Len(t, cfg.UserPolicies, 1)
	p := cfg.UserPolicies[0]
	assert.Equal(t, "default", p.Name)
	require.NotNil(t, p.SignUp)
	assert.False(t, *p.SignUp)
	assert.True(t, p.Quotas.Limit.Unlimited)
}

func TestLoadRejectsUnknownKey(t *testing.T) {
	dir := writeConfigFile(t, "base_ulr: https://typo.example.com\n")
	_, err := Load(envOf(nil), Flags{ConfigDir: dir})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "base_ulr")
}

func TestLoadRejectsBadSize(t *testing.T) {
	dir := writeConfigFile(t, "user_policies:\n  - name: a\n    matches: ['*']\n    quotas:\n      limit: 0\n")
	_, err := Load(envOf(nil), Flags{ConfigDir: dir})
	require.Error(t, err)

	_, err = Load(envOf(nil), Flags{ConfigDir: t.TempDir(), MaxDocumentSize: "-1MB"})
	require.Error(t, err)
}
