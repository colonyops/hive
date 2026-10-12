package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/relay/internal/relay"
)

func writeFile(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "relay.yaml")
	require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	return p
}

func testHash(t *testing.T) string {
	t.Helper()
	h, err := relay.HashPassword("pw")
	require.NoError(t, err)
	return h
}

func TestLoadFile(t *testing.T) {
	hash := testHash(t)
	p := writeFile(t, `
listen: 127.0.0.1:9000
public_url: https://relay.example.com/
store: memory
log_level: debug
retention: 2h
body_bytes: 2048
trusted_proxies: ["10.0.0.0/8"]
allowed_emails: ["*@Example.com", " hayden@colonyops.dev "]
users:
  - email: Pw@Elsewhere.org
    password_hash: "`+hash+`"
smtp:
  url: smtp://relay@mail.example.com:587
  password: env:SMTP_PW
  from: relay@example.com
`)
	cfg, err := load(t, "--config", p)
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1:9000", cfg.Listen)
	assert.Equal(t, "https://relay.example.com", cfg.PublicURL)
	assert.Equal(t, StoreMemory, cfg.Store)
	assert.Equal(t, "debug", cfg.LogLevel)
	assert.Equal(t, 2*time.Hour, cfg.Limits.Retention)
	assert.Equal(t, int64(2048), cfg.Limits.BodyBytes)
	assert.Equal(t, relay.DefaultLimits().Concurrency, cfg.Limits.Concurrency)
	assert.Len(t, cfg.TrustedProxies, 1)
	assert.Equal(t, []string{"*@example.com", "hayden@colonyops.dev"}, cfg.AllowedEmails)
	assert.Equal(t, []User{{Email: "pw@elsewhere.org", PasswordHash: hash}}, cfg.Users)
	assert.Equal(t, map[string]string{"pw@elsewhere.org": hash}, cfg.UserHashes())
	assert.Equal(t, SMTP{URL: "smtp://relay@mail.example.com:587", Password: "env:SMTP_PW", From: "relay@example.com"}, cfg.SMTP)
	assert.True(t, cfg.SMTP.Enabled())
	assert.False(t, cfg.SMTP.IsLog())
}

func TestLoadEnvAndFlagsWinOverFile(t *testing.T) {
	p := writeFile(t, `
listen: 127.0.0.1:9000
store: memory
log_level: debug
allowed_emails: ["*@file.test"]
smtp:
  url: log
  from: file@example.com
`)
	t.Setenv("RELAY_CONFIG", p)
	t.Setenv("RELAY_LOG_LEVEL", "warn")
	t.Setenv("RELAY_ALLOWED_EMAILS", "a@env.test, *@env.test")
	t.Setenv("RELAY_MAIL_FROM", "env@example.com")

	cfg, err := load(t, "--listen", "127.0.0.1:9100")
	require.NoError(t, err)
	assert.Equal(t, p, cfg.ConfigFile)
	assert.Equal(t, "127.0.0.1:9100", cfg.Listen, "a flag wins")
	assert.Equal(t, "warn", cfg.LogLevel, "env wins")
	assert.Equal(t, StoreMemory, cfg.Store, "the file wins over a default")
	assert.Equal(t, []string{"a@env.test", "*@env.test"}, cfg.AllowedEmails)
	assert.Equal(t, SMTP{URL: "log", From: "env@example.com"}, cfg.SMTP)
	assert.True(t, cfg.SMTP.IsLog())
}

func TestLoadFileRejects(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"bad hash", "users: [{email: a@example.com, password_hash: nope}]"},
		{"missing user email", "users: [{password_hash: x}]"},
		{"bad pattern", `allowed_emails: ["[a@example.com"]`},
		{"literal smtp password", "smtp: {url: 'smtp://u@mail.example.com', password: hunter2, from: r@example.com}"},
		{"bad smtp scheme", "smtp: {url: 'http://mail.example.com', from: r@example.com}"},
		{"smtp without from", "smtp: {url: 'smtp://mail.example.com'}"},
		{"smtp user without password", "smtp: {url: 'smtp://u@mail.example.com', from: r@example.com}"},
		{"unknown key", "allowed_email: [a@example.com]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := load(t, "--config", writeFile(t, tt.body))
			assert.Error(t, err)
		})
	}
	_, err := load(t, "--config", filepath.Join(t.TempDir(), "missing.yaml"))
	assert.Error(t, err)
}

func TestLoadDuplicateUser(t *testing.T) {
	hash := testHash(t)
	body := "users:\n  - {email: a@example.com, password_hash: '" + hash + "'}\n  - {email: A@example.com, password_hash: '" + hash + "'}\n"
	_, err := load(t, "--config", writeFile(t, body))
	assert.Error(t, err)
}

func TestReload(t *testing.T) {
	p := writeFile(t, "allowed_emails: [a@example.com]\nlog_level: debug\n")
	cfg, err := load(t, "--config", p)
	require.NoError(t, err)
	require.Equal(t, []string{"a@example.com"}, cfg.AllowedEmails)

	hash := testHash(t)
	require.NoError(t, os.WriteFile(p, []byte("allowed_emails: [b@example.com]\nlog_level: error\nusers: [{email: u@example.com, password_hash: '"+hash+"'}]\n"), 0o600))
	next, err := cfg.Reload()
	require.NoError(t, err)
	assert.Equal(t, []string{"b@example.com"}, next.AllowedEmails)
	assert.Equal(t, []string{"u@example.com"}, next.UserEmails())
	assert.Equal(t, "debug", next.LogLevel, "only the lists reload")

	require.NoError(t, os.WriteFile(p, []byte("allowed_emails: ['[']\n"), 0o600))
	_, err = cfg.Reload()
	assert.Error(t, err)
}
